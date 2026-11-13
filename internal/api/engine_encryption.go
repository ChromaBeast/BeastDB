package api

import (
	"errors"
	"fmt"

	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// ErrEncryptionFlagMismatch: page 0's authenticated flags disagree with how
// the file was opened. Only damage or tampering produces it.
var ErrEncryptionFlagMismatch = errors.New("engine: encryption flag in page 0 does not match the file format")

// Options configures NewEngineWithOptions.
type Options struct {
	// KeyFile enables encryption at rest (ADR 0003): the path of the master key
	// file. Empty means a plaintext database; opening an encrypted one then fails.
	KeyFile string
}

// EncryptionInfo is what the running engine actually does, for status APIs.
type EncryptionInfo struct {
	Enabled bool   `json:"enabled"`
	Cipher  string `json:"cipher,omitempty"`
	KeyID   uint16 `json:"key_id,omitempty"`
}

// Encryption reports whether pages and WAL are encrypted by this engine.
func (e *Engine) Encryption() EncryptionInfo {
	if !e.disk.Encrypted() {
		return EncryptionInfo{}
	}
	return EncryptionInfo{Enabled: true, Cipher: crypt.CipherName, KeyID: e.disk.KeyID()}
}

// KeyringPath is the keyring beside the data file, or "" for a plaintext
// database. Backups copy it (it holds only the wrapped data key).
func (e *Engine) KeyringPath() string {
	if !e.disk.Encrypted() {
		return ""
	}
	return crypt.KeyringPath(e.disk.Path())
}

// VerifyCipher returns a cipher for authenticating copies of the data file
// (nil when plaintext).
func (e *Engine) VerifyCipher() *storage.PageCipher { return e.disk.VerifyCipher() }

// WALCipher returns the WAL cipher (nil when plaintext) for readers of the log.
func (e *Engine) WALCipher() *wal.Cipher { return e.wal.Cipher() }

// NewEngineWithOptions is NewEngine with options.
func NewEngineWithOptions(dbPath, walPath string, poolSize int, opts Options) (*Engine, error) {
	var keys *crypt.Keys
	if opts.KeyFile != "" {
		var err error
		if keys, err = loadKeys(dbPath, opts.KeyFile, 1); err != nil {
			return nil, err
		}
	}
	return openEngineKeys(dbPath, walPath, poolSize, keys)
}

// openEngineKeys opens the engine with already unwrapped keys (nil: plaintext).
func openEngineKeys(dbPath, walPath string, poolSize int, keys *crypt.Keys) (*Engine, error) {
	var pc *storage.PageCipher
	var wc *wal.Cipher
	if keys != nil {
		pc = storage.NewPageCipher(keys.PageAEAD(), keys.ID)
		wc = wal.NewCipher(keys.WALAEAD(), keys.ID)
	}
	disk, err := storage.OpenDiskManagerEncrypted(dbPath, pc)
	if err != nil {
		return nil, err
	}
	return openEngineCipher(disk, dbPath, walPath, poolSize, true, wc)
}

// loadKeys reads the master key and unwraps (or, for a new database, creates)
// the keyring beside dbPath. It never creates a keyring for an existing
// plaintext file and never silently replaces one.
func loadKeys(dbPath, keyFile string, newKeyID uint16) (*crypt.Keys, error) {
	master, err := crypt.ReadMasterKey(keyFile)
	if err != nil {
		return nil, err
	}
	exists, encrypted, err := storage.FileEncrypted(dbPath)
	if err != nil {
		return nil, err
	}
	switch {
	case exists && !encrypted:
		return nil, storage.ErrNotEncrypted
	case exists && !crypt.HasKeyring(dbPath):
		return nil, fmt.Errorf("%w: %s not found", crypt.ErrBadKeyring, crypt.KeyringPath(dbPath))
	case crypt.HasKeyring(dbPath):
		return crypt.OpenKeyring(dbPath, master)
	}
	return crypt.CreateKeyring(dbPath, master, newKeyID)
}
