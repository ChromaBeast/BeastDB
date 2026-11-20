package storage

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
)

// Encrypted page layout (ADR 0003). The last EncReserve bytes of every page
// of an encrypted database hold, in clear:
//
//	[EncDataEnd   : +16] AES-GCM tag
//	[EncDataEnd+16: +28] nonce (random per encryption)
//	[EncDataEnd+28: +30] key ID (uint16 LE)
//	[EncDataEnd+30: +32] magic 0xBE01 (uint16 LE)
//
// Bytes 0..EncDataEnd-1 are the ciphertext of the slotted page. The AAD is
// page ID | key ID, so a page moved to another position fails to open.
const (
	// EncReserve is the tail of each page of an encrypted database.
	EncReserve = 32
	// EncDataEnd is where slotted-page data ends in an encrypted database.
	EncDataEnd = PageSize - EncReserve

	pageMagic    = 0xBE01
	tagSize      = 16
	nonceSize    = 12
	tagOff       = EncDataEnd
	nonceOff     = tagOff + tagSize
	keyIDOff     = nonceOff + nonceSize
	magicOff     = keyIDOff + 2
	pageAADBytes = 10
)

var (
	// ErrEncryptedNoKey: the file is encrypted and no key was supplied.
	ErrEncryptedNoKey = errors.New("storage: database is encrypted; supply -encryption-key-file")
	// ErrNotEncrypted: a key was supplied but the file is plaintext. The engine
	// refuses rather than run unencrypted while the operator expects encryption.
	ErrNotEncrypted = errors.New("storage: database is not encrypted; run `beastctl encrypt` first")
	// ErrPageAuth: a page failed authentication (wrong key, tampering or damage).
	ErrPageAuth = errors.New("storage: page failed authentication (wrong key, tampering or corruption)")
)

// PageCipher seals and opens whole pages. It is safe for concurrent use; the
// AAD buffers come from a pool so neither direction allocates.
type PageCipher struct {
	aead  cipher.AEAD
	keyID uint16
	aads  sync.Pool
}

// NewPageCipher wraps the page AEAD (nonce 12, tag 16) for key keyID.
func NewPageCipher(aead cipher.AEAD, keyID uint16) *PageCipher {
	c := &PageCipher{aead: aead, keyID: keyID}
	c.aads.New = func() any { return new([pageAADBytes]byte) }
	return c
}

// KeyID returns the key generation stamped on every page this cipher seals.
func (c *PageCipher) KeyID() uint16 { return c.keyID }

func (c *PageCipher) setAAD(a *[pageAADBytes]byte, pageID uint64, keyID uint16) []byte {
	binary.LittleEndian.PutUint64(a[0:8], pageID)
	binary.LittleEndian.PutUint16(a[8:10], keyID)
	return a[:]
}

// Seal encrypts plain (PageSize bytes; the tail is ignored) into dst
// (PageSize bytes). dst must not overlap plain.
func (c *PageCipher) Seal(dst, plain []byte, pageID uint64) error {
	nonce := dst[nonceOff : nonceOff+nonceSize]
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	binary.LittleEndian.PutUint16(dst[keyIDOff:], c.keyID)
	binary.LittleEndian.PutUint16(dst[magicOff:], pageMagic)
	a := c.aads.Get().(*[pageAADBytes]byte)
	c.aead.Seal(dst[:0], nonce, plain[:EncDataEnd], c.setAAD(a, pageID, c.keyID))
	c.aads.Put(a)
	return nil
}

// Open decrypts the physical page in place and zeroes its tail. An all-zero
// page, a missing magic or any authentication failure returns ErrPageAuth.
func (c *PageCipher) Open(page []byte, pageID uint64) error {
	if binary.LittleEndian.Uint16(page[magicOff:]) != pageMagic {
		return ErrPageAuth
	}
	keyID := binary.LittleEndian.Uint16(page[keyIDOff:])
	if keyID != c.keyID {
		return ErrPageAuth
	}
	a := c.aads.Get().(*[pageAADBytes]byte)
	_, err := c.aead.Open(page[:0], page[nonceOff:nonceOff+nonceSize], page[:EncDataEnd+tagSize], c.setAAD(a, pageID, keyID))
	c.aads.Put(a)
	if err != nil {
		return ErrPageAuth
	}
	clear(page[EncDataEnd:PageSize])
	return nil
}

// IsEncryptedPage reports whether a physical page carries the encrypted magic.
func IsEncryptedPage(page []byte) bool {
	return len(page) >= PageSize && binary.LittleEndian.Uint16(page[magicOff:]) == pageMagic
}

// AuthenticateFile checks every page of an encrypted data file (for backup
// verification) and returns page 0's metadata. It needs only the file and the
// cipher, not a running engine. Any page that fails to open, including an
// all-zero page inside the file, returns ErrPageAuth.
func AuthenticateFile(path string, pc *PageCipher) (MetaData, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return MetaData{}, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return MetaData{}, 0, err
	}
	if info.Size()%PageSize != 0 || info.Size() == 0 {
		return MetaData{}, 0, fmt.Errorf("%w: file size %d is not a whole number of pages", ErrPageAuth, info.Size())
	}
	n := uint64(info.Size() / PageSize)
	buf := make([]byte, PageSize)
	var meta MetaData
	for id := uint64(0); id < n; id++ {
		if _, err := f.ReadAt(buf, int64(id)*PageSize); err != nil {
			return MetaData{}, 0, err
		}
		if err := pc.Open(buf, id); err != nil {
			return MetaData{}, 0, fmt.Errorf("%w: page %d", err, id)
		}
		if id == MetaPageID {
			if meta, err = DecodeMeta(buf); err != nil {
				return MetaData{}, 0, err
			}
		}
	}
	return meta, n, nil
}
