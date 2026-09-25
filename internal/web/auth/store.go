package auth

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"time"
)

// UserStore manages system users stored in BeastDB's reserved _sys:user: keyspace.
// It wraps the Engine interface to avoid importing the api package directly.
type UserStore struct {
	engine EngineWriter
}

// EngineWriter is the minimal Engine interface required by the user store.
type EngineWriter interface {
	Put(key uint64, value []byte) error
	Get(key uint64) ([]byte, bool, error)
	Delete(key uint64) error
}

// NewUserStore creates a UserStore backed by the given engine.
func NewUserStore(e EngineWriter) *UserStore {
	return &UserStore{engine: e}
}

// sysUserKey hashes "_sys:user:<username>" into a stable uint64 B+ Tree key.
// FNV-1a is used for speed — collisions are astronomically unlikely for usernames.
func sysUserKey(username string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("_sys:user:"))
	_, _ = h.Write([]byte(username))
	return h.Sum64()
}

// CreateUser hashes the password and persists the user record.
// Returns an error if the username already exists.
func (s *UserStore) CreateUser(username, password string, role Role) error {
	key := sysUserKey(username)

	if _, exists, err := s.engine.Get(key); err != nil {
		return fmt.Errorf("checking existence: %w", err)
	} else if exists {
		return fmt.Errorf("user %q already exists", username)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	u := &User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	data, err := u.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling user: %w", err)
	}

	return s.engine.Put(key, data)
}

// Authenticate verifies credentials and returns the User on success.
func (s *UserStore) Authenticate(username, password string) (*User, error) {
	data, exists, err := s.engine.Get(sysUserKey(username))
	if err != nil {
		return nil, fmt.Errorf("reading user: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("invalid credentials")
	}

	u, err := UnmarshalUser(data)
	if err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}

	ok, err := VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		return nil, fmt.Errorf("invalid credentials")
	}

	return u, nil
}

// GetUser retrieves a user by username.
func (s *UserStore) GetUser(username string) (*User, error) {
	data, exists, err := s.engine.Get(sysUserKey(username))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("user %q not found", username)
	}
	return UnmarshalUser(data)
}

// DeleteUser removes the user record from the engine.
func (s *UserStore) DeleteUser(username string) error {
	return s.engine.Delete(sysUserKey(username))
}

// uint64ToBytes encodes a uint64 as little-endian bytes (used for engine keys).
func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}
