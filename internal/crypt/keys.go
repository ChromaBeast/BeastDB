// Package crypt holds the key hierarchy for encryption at rest (ADR 0003): a
// master key supplied by the operator wraps a random per-database data key
// (DEK) kept in <db>.keyring, and the DEK yields separate page and WAL keys.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// KeySize is the size of the master key, the DEK and every derived key.
	KeySize = 32
	// CipherName is recorded in the keyring and reported to Studio.
	CipherName = "aes-256-gcm"
)

var (
	// ErrWrongKey means the master key did not unwrap the database's data key.
	ErrWrongKey = errors.New("crypt: master key does not match this database")
	// ErrBadKeyFile means the key file is not 32 raw bytes or 64 hex characters.
	ErrBadKeyFile = errors.New("crypt: key file must hold 32 raw bytes or 64 hex characters")
	// ErrKeyFilePerm means the key file is readable by group or others.
	ErrKeyFilePerm = errors.New("crypt: key file must not be accessible by group or others (chmod 600)")
	// ErrBadKeyring means the keyring file is missing, damaged or of an unknown version.
	ErrBadKeyring = errors.New("crypt: keyring is missing or damaged")
)

// Keys are the unwrapped keys for one open database.
type Keys struct {
	ID   uint16
	page cipher.AEAD
	wal  cipher.AEAD
}

// PageAEAD returns the AEAD sealing data, journal and spill pages.
func (k *Keys) PageAEAD() cipher.AEAD { return k.page }

// WALAEAD returns the AEAD sealing WAL frames.
func (k *Keys) WALAEAD() cipher.AEAD { return k.wal }

// KeyringPath names the keyring that belongs to a data file.
func KeyringPath(dbPath string) string { return dbPath + ".keyring" }

// HasKeyring reports whether the data file has a keyring beside it.
func HasKeyring(dbPath string) bool {
	_, err := os.Stat(KeyringPath(dbPath))
	return err == nil
}

// ReadMasterKey loads a master key from a file of 32 raw bytes or 64 hex
// characters (surrounding whitespace ignored). On Unix the file must not be
// accessible by group or others.
func ReadMasterKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("crypt: read key file: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s has mode %04o", ErrKeyFilePerm, path, info.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypt: read key file: %w", err)
	}
	if len(b) == KeySize {
		return b, nil
	}
	if k, err := hex.DecodeString(strings.TrimSpace(string(b))); err == nil && len(k) == KeySize {
		return k, nil
	}
	return nil, ErrBadKeyFile
}

// GenerateKeyFile writes a new random master key (hex) to path with mode 0600.
// It refuses to overwrite an existing file.
func GenerateKeyFile(path string) error {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(hex.EncodeToString(k) + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

type keyringFile struct {
	Version    int    `json:"version"`
	KeyID      uint16 `json:"key_id"`
	Cipher     string `json:"cipher"`
	WrappedDEK string `json:"wrapped_dek"` // base64(nonce | ciphertext | tag)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func derive(secret []byte, info string) ([]byte, error) {
	return hkdf.Key(sha256.New, secret, nil, info, KeySize)
}

func wrapAAD(id uint16) []byte { return []byte{'b', 'd', 'b', 'k', 'r', 1, byte(id), byte(id >> 8)} }

func keysFromDEK(dek []byte, id uint16) (*Keys, error) {
	pk, err := derive(dek, "beastdb page v1")
	if err != nil {
		return nil, err
	}
	wk, err := derive(dek, "beastdb wal v1")
	if err != nil {
		return nil, err
	}
	pa, err := newAEAD(pk)
	if err != nil {
		return nil, err
	}
	wa, err := newAEAD(wk)
	if err != nil {
		return nil, err
	}
	return &Keys{ID: id, page: pa, wal: wa}, nil
}

// CreateKeyring generates a random DEK, wraps it under master and writes
// <dbPath>.keyring (temp file, fsync, rename). It refuses to replace an
// existing keyring. keyID must be non-zero.
func CreateKeyring(dbPath string, master []byte, keyID uint16) (*Keys, error) {
	if len(master) != KeySize || keyID == 0 {
		return nil, errors.New("crypt: invalid master key or key id")
	}
	path := KeyringPath(dbPath)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("crypt: %s already exists", path)
	}
	dek := make([]byte, KeySize)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	kek, err := derive(master, "beastdb kek v1")
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	wrapped := aead.Seal(nonce, nonce, dek, wrapAAD(keyID))
	b, _ := json.MarshalIndent(keyringFile{Version: 1, KeyID: keyID, Cipher: CipherName,
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapped)}, "", "  ")

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return keysFromDEK(dek, keyID)
}

// OpenKeyring unwraps the DEK stored beside dbPath. A master key that does not
// match returns ErrWrongKey.
func OpenKeyring(dbPath string, master []byte) (*Keys, error) {
	return OpenKeyringFile(KeyringPath(dbPath), master)
}

// OpenKeyringFile is OpenKeyring for an explicit keyring path (backup bundles).
func OpenKeyringFile(path string, master []byte) (*Keys, error) {
	if len(master) != KeySize {
		return nil, ErrBadKeyFile
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadKeyring, err)
	}
	var kf keyringFile
	if err := json.Unmarshal(b, &kf); err != nil || kf.Version != 1 || kf.Cipher != CipherName || kf.KeyID == 0 {
		return nil, ErrBadKeyring
	}
	wrapped, err := base64.StdEncoding.DecodeString(kf.WrappedDEK)
	if err != nil {
		return nil, ErrBadKeyring
	}
	kek, err := derive(master, "beastdb kek v1")
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrBadKeyring
	}
	n := aead.NonceSize()
	dek, err := aead.Open(nil, wrapped[:n], wrapped[n:], wrapAAD(kf.KeyID))
	if err != nil {
		return nil, ErrWrongKey
	}
	return keysFromDEK(dek, kf.KeyID)
}

// KeyringKeyID reads the key ID from the keyring beside dbPath without a key.
func KeyringKeyID(dbPath string) (uint16, error) {
	b, err := os.ReadFile(KeyringPath(dbPath))
	if err != nil {
		return 0, ErrBadKeyring
	}
	var kf keyringFile
	if json.Unmarshal(b, &kf) != nil || kf.KeyID == 0 {
		return 0, ErrBadKeyring
	}
	return kf.KeyID, nil
}
