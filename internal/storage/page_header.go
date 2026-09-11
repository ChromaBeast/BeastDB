package storage

import (
	"encoding/binary"
)

const (
	// PageSize is the standard hardware-aligned disk block size (4KB).
	PageSize = 4096

	// PageHeaderSize is the 24-byte metadata header at the top of every page.
	// PageID (8B) + LSN (8B) + SlotCount (2B) + Lower (2B) + Upper (2B) + Flags (2B) = 24B.
	PageHeaderSize = 24

	// SlotSize is the 4-byte entry in the slot directory: Offset (2B) + Length (2B).
	SlotSize = 4
)

// PageHeader represents the parsed metadata of a 4KB disk page.
type PageHeader struct {
	PageID    uint64
	LSN       uint64
	SlotCount uint16
	Lower     uint16
	Upper     uint16
	Flags     uint16
}

// ReadPageHeader decodes the 24-byte header directly from a page slice without allocations.
func ReadPageHeader(page []byte) PageHeader {
	return PageHeader{
		PageID:    binary.LittleEndian.Uint64(page[0:8]),
		LSN:       binary.LittleEndian.Uint64(page[8:16]),
		SlotCount: binary.LittleEndian.Uint16(page[16:18]),
		Lower:     binary.LittleEndian.Uint16(page[18:20]),
		Upper:     binary.LittleEndian.Uint16(page[20:22]),
		Flags:     binary.LittleEndian.Uint16(page[22:24]),
	}
}

// WritePageHeader writes the 24-byte header directly into the page slice.
func WritePageHeader(page []byte, h PageHeader) {
	binary.LittleEndian.PutUint64(page[0:8], h.PageID)
	binary.LittleEndian.PutUint64(page[8:16], h.LSN)
	binary.LittleEndian.PutUint16(page[16:18], h.SlotCount)
	binary.LittleEndian.PutUint16(page[18:20], h.Lower)
	binary.LittleEndian.PutUint16(page[20:22], h.Upper)
	binary.LittleEndian.PutUint16(page[22:24], h.Flags)
}
