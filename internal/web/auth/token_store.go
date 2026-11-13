package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

const TokenPartitionPrefix = uint8(0x05)

// APIToken represents a persistent API access token stored in partition 0x05.
type APIToken struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	TokenHash   string    `json:"token_hash"`
	MaskedToken string    `json:"masked_token"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	// ExpiresAt is optional; nil (or absent in older stored tokens) means no expiry.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// LastUsedAt is filled in by List from in-memory tracking: the last time the
	// token authenticated a request since this process started. It is never
	// persisted (authenticating must not write to the engine), so it is nil for
	// tokens not used since the last restart.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Expired reports whether the token has an expiry that is at or before now.
func (t APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// TokenEngine is the storage interface needed for token persistence.
type TokenEngine interface {
	EngineWriter
	ScanRange(startKey, endKey uint64) ([]TokenRecord, error)
}

// TokenRecord is a key-value record from scan.
type TokenRecord struct {
	Key   uint64
	Value []byte
}

// TokenStore coordinates token persistence and zero-alloc in-memory validation.
type TokenStore struct {
	engine     EngineWriter
	scanFn     func(start, end uint64) ([]TokenRecord, error)
	mu         sync.RWMutex
	tokenIndex map[string]APIToken // tokenHash -> APIToken
	lastUsed   sync.Map            // tokenHash -> *atomic.Int64 (unix nanoseconds)
}

// NewTokenStore initializes TokenStore and loads existing tokens.
func NewTokenStore(e EngineWriter, scanFn func(start, end uint64) ([]TokenRecord, error)) *TokenStore {
	ts := &TokenStore{
		engine:     e,
		scanFn:     scanFn,
		tokenIndex: make(map[string]APIToken),
	}
	_ = ts.Reload()
	return ts
}

func tokenKey(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("_sys:token:" + id))
	return (uint64(TokenPartitionPrefix) << 56) | (h.Sum64() & 0x00ffffffffffffff)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Create generates a new API token, saves it to partition 0x05, and activates it.
func (ts *TokenStore) Create(name, role, createdBy string) (string, *APIToken, error) {
	return ts.CreateWithExpiry(name, role, createdBy, nil)
}

// CreateWithExpiry is Create with an optional expiry time (nil = never expires).
func (ts *TokenStore) CreateWithExpiry(name, role, createdBy string, expiresAt *time.Time) (string, *APIToken, error) {
	idBytes := make([]byte, 4)
	tokenBytes := make([]byte, 20)
	if _, err := rand.Read(idBytes); err != nil {
		return "", nil, err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, err
	}

	id := "tok_" + hex.EncodeToString(idBytes)
	rawToken := "bst_live_" + hex.EncodeToString(tokenBytes)
	th := hashToken(rawToken)
	masked := "bst_live_..." + rawToken[len(rawToken)-4:]

	tok := &APIToken{
		ID:          id,
		Name:        name,
		TokenHash:   th,
		MaskedToken: masked,
		Role:        role,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   createdBy,
	}
	if expiresAt != nil {
		e := expiresAt.UTC()
		tok.ExpiresAt = &e
	}

	data, err := json.Marshal(tok)
	if err != nil {
		return "", nil, err
	}

	if err := ts.engine.Put(tokenKey(id), data); err != nil {
		return "", nil, err
	}

	ts.mu.Lock()
	ts.tokenIndex[th] = *tok
	ts.mu.Unlock()

	return rawToken, tok, nil
}

// List returns all active API tokens.
func (ts *TokenStore) List() []APIToken {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	list := make([]APIToken, 0, len(ts.tokenIndex))
	for h, tok := range ts.tokenIndex {
		if v, ok := ts.lastUsed.Load(h); ok {
			if ns := v.(*atomic.Int64).Load(); ns > 0 {
				t := time.Unix(0, ns).UTC()
				tok.LastUsedAt = &t
			}
		}
		list = append(list, tok)
	}
	return list
}

// Delete revokes an API token by ID from the database and in-memory index.
func (ts *TokenStore) Delete(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for h, tok := range ts.tokenIndex {
		if tok.ID == id {
			delete(ts.tokenIndex, h)
			ts.lastUsed.Delete(h)
			break
		}
	}
	return ts.engine.Delete(tokenKey(id))
}

// Validate checks whether a given raw token is valid in < 100ns without disk I/O.
func (ts *TokenStore) Validate(raw string) bool {
	_, ok := ts.ValidateRole(raw)
	return ok
}

// ValidateRole returns the token's role if it exists and has not expired.
// Lookup is by SHA-256 hash of the presented token (map lookup on a hash, so
// no raw-secret comparison timing leak).
func (ts *TokenStore) ValidateRole(raw string) (string, bool) {
	th := hashToken(raw)
	ts.mu.RLock()
	tok, exists := ts.tokenIndex[th]
	ts.mu.RUnlock()
	now := time.Now()
	if !exists || tok.Expired(now) {
		return "", false
	}
	ts.touch(th, now)
	return tok.Role, true
}

// touch records a successful authentication. Pure memory: one atomic store, and
// an allocation only the first time a token is seen.
func (ts *TokenStore) touch(hash string, now time.Time) {
	v, ok := ts.lastUsed.Load(hash)
	if !ok {
		v, _ = ts.lastUsed.LoadOrStore(hash, new(atomic.Int64))
	}
	v.(*atomic.Int64).Store(now.UnixNano())
}

// Reload repopulates the in-memory token cache from partition 0x05.
func (ts *TokenStore) Reload() error {
	if ts.scanFn == nil {
		return nil
	}
	start := uint64(TokenPartitionPrefix) << 56
	end := start | 0x00ffffffffffffff

	records, err := ts.scanFn(start, end)
	if err != nil {
		return err
	}

	newIdx := make(map[string]APIToken)
	for _, rec := range records {
		var tok APIToken
		if err := json.Unmarshal(rec.Value, &tok); err == nil && tok.TokenHash != "" {
			newIdx[tok.TokenHash] = tok
		}
	}

	ts.mu.Lock()
	ts.tokenIndex = newIdx
	ts.mu.Unlock()
	return nil
}
