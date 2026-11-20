package api

import (
	"errors"
	"fmt"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// ErrEncryptionFlagMismatch: page 0's authenticated flags disagree with how
// the file was opened. Only damage or tampering produces it.
var ErrEncryptionFlagMismatch = errors.New("engine: encryption flag in page 0 does not match the file format")

// EncryptionInfo is what the running engine actually does, for status APIs.
type EncryptionInfo struct {
	Enabled bool   `json:"enabled"`
	Cipher  string `json:"cipher,omitempty"`
	KeyID   uint16 `json:"key_id,omitempty"`
	// KeySource is the kind of master key source in use ("file", "env" or
	// "exec"), never its name or value. Empty when the engine was opened with
	// already unwrapped keys.
	KeySource string `json:"key_source,omitempty"`
}

// Encryption reports whether pages and WAL are encrypted by this engine.
func (e *Engine) Encryption() EncryptionInfo {
	if !e.disk.Encrypted() {
		return EncryptionInfo{}
	}
	return EncryptionInfo{Enabled: true, Cipher: crypt.CipherName, KeyID: e.disk.KeyID(), KeySource: e.keySourceKind()}
}

func (e *Engine) keySourceKind() string {
	if e.opts.KeyFile == "" {
		return ""
	}
	return crypt.SourceKind(e.opts.KeyFile)
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

// AuditCipher returns the cipher for the audit log (nil when plaintext). The
// server opens the audit log with it so the log is sealed whenever the
// database is.
func (e *Engine) AuditCipher() *audit.Cipher { return e.opts.auditCipher }

// WALCipher returns the WAL cipher (nil when plaintext) for readers of the log.
func (e *Engine) WALCipher() *wal.Cipher { return e.wal.Cipher() }

// openEngineKeys opens the engine with already unwrapped keys (nil: plaintext).
func openEngineKeys(dbPath, walPath string, poolSize int, keys *crypt.Keys, opts Options) (*Engine, error) {
	var pc *storage.PageCipher
	if keys != nil {
		pc = storage.NewPageCipher(keys.PageAEAD(), keys.ID)
		opts.walCipher = wal.NewCipher(keys.WALAEAD(), keys.ID)
		opts.auditCipher = audit.NewCipher(keys.AuditAEAD(), keys.ID)
	} else {
		opts.walCipher = nil
		opts.auditCipher = nil
	}
	disk, err := storage.OpenDiskManagerEncrypted(dbPath, pc)
	if err != nil {
		return nil, err
	}
	return openEngine(disk, dbPath, walPath, poolSize, true, opts)
}

// loadKeys reads the master key and unwraps (or, for a new database, creates)
// the keyring beside dbPath. It never creates a keyring for an existing
// plaintext file and never silently replaces one.
func loadKeys(dbPath, keyFile string, newKeyID uint16) (*crypt.Keys, error) {
	master, err := crypt.ReadMasterKey(keyFile)
	if err != nil {
		return nil, err
	}
	defer crypt.Zero(master)
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
