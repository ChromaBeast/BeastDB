package auth

import (
	"encoding/json"
	"time"

	"golang.org/x/crypto/argon2"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// Role defines the permission level of a user.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

// User holds the persisted identity record stored in BeastDB's keyspace.
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"` // argon2id encoded string
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// argon2Params are the cost parameters — tuned for ~300ms on commodity hardware.
var argon2Params = struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}{time: 3, memory: 64 * 1024, threads: 4, keyLen: 32}

// HashPassword produces a self-contained argon2id encoded string including the salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	p := argon2Params
	hash := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks a plaintext password against a stored argon2id hash.
// Uses constant-time comparison to prevent timing oracle attacks.
func VerifyPassword(password, encoded string) (bool, error) {
	var mem, tim uint32
	var threads uint8
	var saltB64, hashB64 string

	_, err := fmt.Sscanf(encoded,
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s",
		&mem, &tim, &threads, &saltB64)
	if err != nil {
		return false, fmt.Errorf("malformed hash: %w", err)
	}

	// saltB64 contains both salt$hash — split on last '$'
	for i := len(saltB64) - 1; i >= 0; i-- {
		if saltB64[i] == '$' {
			hashB64 = saltB64[i+1:]
			saltB64 = saltB64[:i]
			break
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return false, fmt.Errorf("decoding salt: %w", err)
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return false, fmt.Errorf("decoding hash: %w", err)
	}

	candidate := argon2.IDKey([]byte(password), salt, tim, mem, threads, uint32(len(storedHash)))
	return subtle.ConstantTimeCompare(candidate, storedHash) == 1, nil
}

// Marshal serializes a User to JSON bytes for storage in BeastDB.
func (u *User) Marshal() ([]byte, error) {
	return json.Marshal(u)
}

// UnmarshalUser deserializes a User from JSON bytes read from BeastDB.
func UnmarshalUser(data []byte) (*User, error) {
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
