// Package crypt holds the key hierarchy for encryption at rest (ADR 0003): a
// master key supplied by the operator wraps a random per-database data key
// (DEK) kept in <db>.keyring, and the DEK yields separate page and WAL keys.
package crypt

import (
	"bytes"
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
	ID    uint16
	page  cipher.AEAD
	wal   cipher.AEAD
	audit cipher.AEAD
}

// PageAEAD returns the AEAD sealing data, journal and spill pages.
func (k *Keys) PageAEAD() cipher.AEAD { return k.page }

// AuditAEAD returns the AEAD sealing audit log records.
func (k *Keys) AuditAEAD() cipher.AEAD { return k.audit }

// WALAEAD returns the AEAD sealing WAL frames.
func (k *Keys) WALAEAD() cipher.AEAD { return k.wal }

// KeyringPath names the keyring that belongs to a data file.
func KeyringPath(dbPath string) string { return dbPath + ".keyring" }

// HasKeyring reports whether the data file has a keyring beside it.
func HasKeyring(dbPath string) bool {
	_, err := os.Stat(KeyringPath(dbPath))
	return err == nil
}

// readKeyFile loads a master key from a file of 32 raw bytes or 64 hex
// characters (surrounding whitespace ignored). On Unix the file must not be
// accessible by group or others.
func readKeyFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("crypt: read key file: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s has mode %04o", ErrKeyFilePerm, path, info.Mode().Perm())
	}
	if info.Size() > maxKeyOutput {
		return nil, ErrBadKeyFile
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypt: read key file: %w", err)
	}
	return parseKey(b)
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
	defer Zero(pk)
	defer Zero(wk)
	ak, err := derive(dek, "beastdb audit v1")
	if err != nil {
		return nil, err
	}
	defer Zero(ak)
	aa, err := newAEAD(ak)
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
	return &Keys{ID: id, page: pa, wal: wa, audit: aa}, nil
}

// wrapDEK seals dek under a KEK derived from master and returns the keyring
// document.
func wrapDEK(master, dek []byte, keyID uint16) (keyringFile, error) {
	kek, err := derive(master, "beastdb kek v1")
	if err != nil {
		return keyringFile{}, err
	}
	defer Zero(kek)
	aead, err := newAEAD(kek)
	if err != nil {
		return keyringFile{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return keyringFile{}, err
	}
	wrapped := aead.Seal(nonce, nonce, dek, wrapAAD(keyID))
	return keyringFile{Version: 1, KeyID: keyID, Cipher: CipherName,
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapped)}, nil
}

// unwrapDEK opens the keyring document with master. The caller zeroes the DEK.
func unwrapDEK(kf keyringFile, master []byte) ([]byte, error) {
	if len(master) != KeySize {
		return nil, ErrBadKeyFile
	}
	if kf.Version != 1 || kf.Cipher != CipherName || kf.KeyID == 0 {
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
	defer Zero(kek)
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	n := aead.NonceSize()
	if len(wrapped) < n+aead.Overhead() {
		return nil, ErrBadKeyring
	}
	dek, err := aead.Open(nil, wrapped[:n], wrapped[n:], wrapAAD(kf.KeyID))
	if err != nil {
		return nil, ErrWrongKey
	}
	return dek, nil
}

func readKeyringDoc(path string) (keyringFile, error) {
	var kf keyringFile
	b, err := os.ReadFile(path)
	if err != nil {
		return kf, fmt.Errorf("%w: %v", ErrBadKeyring, err)
	}
	if err := json.Unmarshal(b, &kf); err != nil {
		return kf, ErrBadKeyring
	}
	return kf, nil
}

// writeKeyringDoc writes the document to a temp file, fsyncs it, renames it
// over path and fsyncs the directory. A crash leaves either the old file or
// the new one, never a mix; a leftover .tmp is ignored and overwritten.
func writeKeyringDoc(path string, kf keyringFile) error {
	b, _ := json.MarshalIndent(kf, "", "  ")
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
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
	defer Zero(dek)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	kf, err := wrapDEK(master, dek, keyID)
	if err != nil {
		return nil, err
	}
	if err := writeKeyringDoc(path, kf); err != nil {
		return nil, err
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
	kf, err := readKeyringDoc(path)
	if err != nil {
		return nil, err
	}
	dek, err := unwrapDEK(kf, master)
	if err != nil {
		return nil, err
	}
	defer Zero(dek)
	return keysFromDEK(dek, kf.KeyID)
}

// Rewrap changes the master key of an encrypted database without touching a
// single data page: it unwraps the DEK with oldMaster, wraps the same DEK under
// newMaster and replaces <dbPath>.keyring atomically. The DEK and key ID do not
// change, so every page, WAL frame and existing backup stays readable with the
// new master key. Until the rename the old master key still opens the
// keyring; after it only the new one does. The new keyring is read back and
// opened with newMaster before the call returns.
//
// Rewrap protects against a later leak of the old master key only for copies
// of the keyring made after the swap: anyone who kept an older keyring and the
// old master key still holds the DEK. Replacing the DEK needs a data rewrite
// (beastctl rekey --rotate-data-key).
func Rewrap(dbPath string, oldMaster, newMaster []byte) error {
	if len(newMaster) != KeySize {
		return ErrBadKeyFile
	}
	path := KeyringPath(dbPath)
	kf, err := readKeyringDoc(path)
	if err != nil {
		return err
	}
	dek, err := unwrapDEK(kf, oldMaster)
	if err != nil {
		return err
	}
	defer Zero(dek)
	nkf, err := wrapDEK(newMaster, dek, kf.KeyID)
	if err != nil {
		return err
	}
	// Prove the new document unwraps to the same DEK before it replaces the old.
	chk, err := unwrapDEK(nkf, newMaster)
	if err != nil || !bytes.Equal(chk, dek) {
		Zero(chk)
		return errors.New("crypt: internal error: rewrapped key does not round-trip; keyring left unchanged")
	}
	Zero(chk)
	if err := writeKeyringDoc(path, nkf); err != nil {
		return err
	}
	k, err := OpenKeyring(dbPath, newMaster)
	if err != nil {
		return fmt.Errorf("crypt: rewrapped keyring does not open with the new key: %w", err)
	}
	if k.ID != kf.KeyID {
		return ErrBadKeyring
	}
	return nil
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
