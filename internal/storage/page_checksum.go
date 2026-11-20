package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// Every page in a version 2 data file ends with a 4-byte trailer holding a
// CRC-32C of the page ID and the rest of the page. The disk manager stamps it
// whenever a page leaves memory and verifies it whenever a page is read back,
// so a damaged or misdirected page surfaces as ErrPageChecksum instead of
// being parsed. Page layouts must keep their bytes below PageDataEnd.
const (
	PageTrailerSize = 4
	PageDataEnd     = PageSize - PageTrailerSize
)

// ErrPageChecksum reports a page whose contents do not match its checksum.
var ErrPageChecksum = errors.New("page checksum mismatch")

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func pageChecksum(pageID uint64, page []byte) uint32 {
	var id [8]byte
	binary.LittleEndian.PutUint64(id[:], pageID)
	return crc32.Update(crc32.Checksum(id[:], castagnoli), castagnoli, page[:PageDataEnd])
}

// stampPage writes page's checksum into its trailer.
func stampPage(pageID uint64, page []byte) {
	binary.LittleEndian.PutUint32(page[PageDataEnd:PageSize], pageChecksum(pageID, page))
}

// checksumValid reports whether page carries the checksum for pageID.
func checksumValid(pageID uint64, page []byte) bool {
	return binary.LittleEndian.Uint32(page[PageDataEnd:PageSize]) == pageChecksum(pageID, page)
}

// verifyPage accepts a page with a valid checksum, or one that is zero
// throughout: a page that was allocated but never written reads as zeros.
func verifyPage(pageID uint64, page []byte) error {
	if checksumValid(pageID, page) || allZero(page[:PageSize]) {
		return nil
	}
	return fmt.Errorf("page %d: %w", pageID, ErrPageChecksum)
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// VerifyPageImage checks one page image read straight from a version 2 data
// file (for example by a backup verifier).
func VerifyPageImage(pageID uint64, page []byte) error {
	if len(page) < PageSize {
		return ErrDiskBufferTooSmall
	}
	return verifyPage(pageID, page)
}
