package audit

import (
	"bufio"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Encrypted audit files (ADR 0003): every line is one sealed record,
//
//	BAE1 <keyID> <base64(nonce | AES-256-GCM(JSON entry) | tag)>
//
// The sealed JSON is the same Entry the plaintext log holds, including its Prev
// and Hash fields, so the hash chain is verified exactly as before, after
// decryption. The AAD is the key ID and the record's line number, so a line
// cannot be moved to another position. A file is either wholly plaintext (every
// line starts with "{") or wholly sealed; a mix is refused, otherwise anyone
// could append forged plaintext lines to a sealed log.
//
// Not covered: removing the newest records, or replacing the whole file with an
// older sealed copy, still verifies (there is no trusted counter). Record
// timestamps, line count and line lengths are visible.

const sealedPrefix = "BAE1 "

var (
	// ErrSealed means the audit log is encrypted and no key was supplied.
	ErrSealed = errors.New("audit log is encrypted; start the server with the database's encryption key, or run beastctl decrypt")
	// ErrMixedFormat means sealed and plaintext lines are mixed in one file.
	ErrMixedFormat = errors.New("audit log mixes encrypted and plaintext lines")
	// ErrAuth means a sealed record did not authenticate.
	ErrAuth = errors.New("audit record failed authentication (wrong key or tampered)")
)

// Cipher seals audit records. A nil *Cipher means plaintext.
type Cipher struct {
	aead  cipher.AEAD
	keyID uint16
}

// NewCipher wraps an AEAD (the audit subkey from crypt.Keys) and its key ID.
func NewCipher(aead cipher.AEAD, keyID uint16) *Cipher { return &Cipher{aead: aead, keyID: keyID} }

func lineAAD(keyID uint16, line uint64) []byte {
	b := make([]byte, 10)
	binary.LittleEndian.PutUint16(b, keyID)
	binary.LittleEndian.PutUint64(b[2:], line)
	return b
}

func (c *Cipher) seal(e Entry, line uint64) ([]byte, error) {
	pt, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := c.aead.Seal(nonce, nonce, pt, lineAAD(c.keyID, line))
	return []byte(sealedPrefix + strconv.Itoa(int(c.keyID)) + " " + base64.StdEncoding.EncodeToString(ct)), nil
}

func (c *Cipher) open(text string, line uint64) (Entry, error) {
	var e Entry
	rest := strings.TrimPrefix(text, sealedPrefix)
	sp := strings.IndexByte(rest, ' ')
	if sp < 0 {
		return e, fmt.Errorf("line %d: malformed encrypted record", line)
	}
	id, err := strconv.Atoi(rest[:sp])
	if err != nil || id != int(c.keyID) {
		return e, fmt.Errorf("line %d: %w (record uses key %s)", line, ErrAuth, rest[:sp])
	}
	raw, err := base64.StdEncoding.DecodeString(rest[sp+1:])
	n := c.aead.NonceSize()
	if err != nil || len(raw) < n+c.aead.Overhead() {
		return e, fmt.Errorf("line %d: malformed encrypted record", line)
	}
	pt, err := c.aead.Open(nil, raw[:n], raw[n:], lineAAD(c.keyID, line))
	if err != nil {
		return e, fmt.Errorf("line %d: %w", line, ErrAuth)
	}
	if err := json.Unmarshal(pt, &e); err != nil {
		return e, fmt.Errorf("line %d: %w", line, err)
	}
	return e, nil
}

// encode returns the stored form of e as line number line (1-based).
func (c *Cipher) encode(e Entry, line uint64) ([]byte, error) {
	if c == nil {
		return json.Marshal(e)
	}
	return c.seal(e, line)
}

// fileFormat reports whether an existing file is sealed, from its first
// non-empty line. An empty or missing file is neither.
func fileFormat(path string) (sealed, empty bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, true, nil
		}
		return false, false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" {
			continue
		}
		return strings.HasPrefix(t, sealedPrefix), false, nil
	}
	return false, true, sc.Err()
}

// Sealed reports whether this log writes encrypted records.
func (l *Log) Sealed() bool { return l != nil && l.c != nil }

// MigrationSuffix names the temp file used while converting a log's format.
const MigrationSuffix = ".migrating"

// PrepareConvert reads dir/audit.log with from, verifies its chain, and writes
// the same records, in the format of to, to dir/audit.log.migrating (fsynced).
// Neither argument may be nil-and-equal in effect: the caller decides when a
// conversion is needed. The original is untouched; InstallConverted swaps it
// in. It returns false when there is no audit log to convert.
func PrepareConvert(dir string, from, to *Cipher) (bool, error) {
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.Remove(path + MigrationSuffix)
		return false, nil
	}
	entries, err := readAll(path, from)
	if err == nil {
		err = checkChain(entries)
	}
	if err != nil {
		return false, fmt.Errorf("audit log %s: %w", path, err)
	}
	return true, writeAll(path+MigrationSuffix, entries, to)
}

// InstallConverted atomically replaces dir/audit.log with the prepared file.
func InstallConverted(dir string) error {
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path + MigrationSuffix); err != nil {
		return nil
	}
	if err := os.Rename(path+MigrationSuffix, path); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

func writeAll(path string, entries []Entry, c *Cipher) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for i, e := range entries {
		b, err := c.encode(e, uint64(i+1))
		if err == nil {
			_, err = w.Write(append(b, '\n'))
		}
		if err != nil {
			f.Close()
			_ = os.Remove(path)
			return err
		}
	}
	err = w.Flush()
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// recoverInterrupted finishes or discards a conversion that was interrupted
// between the data-file swap and the audit-file swap (see api.MigrateEncryption):
// the prepared file replaces the log only if the log no longer opens with c and
// the prepared one does.
func recoverInterrupted(dir string, c *Cipher) {
	path := filepath.Join(dir, FileName)
	tmp := path + MigrationSuffix
	if _, err := os.Stat(tmp); err != nil {
		return
	}
	if _, _, err := verifyFile(path, c); err != nil {
		if _, _, terr := verifyFile(tmp, c); terr == nil {
			if os.Rename(tmp, path) == nil {
				syncDir(dir)
				return
			}
		}
	}
	_ = os.Remove(tmp)
}
