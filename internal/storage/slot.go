package storage

import (
	"encoding/binary"
)

// Slot is a 4-byte entry in the slot directory pointing to variable-length tuple data.
// If Offset == 0, the slot represents a deleted tuple (Tombstone).
type Slot struct {
	Offset uint16
	Length uint16
}

// RID (Record Identifier) uniquely addresses any row in the database by Page and Slot.
// External indexes (like B+ Trees) reference RIDs so physical tuple moves never invalidate indexes.
type RID struct {
	PageID uint64
	SlotID uint16
}

// ReadSlot reads the slot at slotIdx directly from the page slice without allocations.
func ReadSlot(page []byte, slotIdx uint16) Slot {
	offset := PageHeaderSize + int(slotIdx)*SlotSize
	return Slot{
		Offset: binary.LittleEndian.Uint16(page[offset : offset+2]),
		Length: binary.LittleEndian.Uint16(page[offset+2 : offset+4]),
	}
}

// WriteSlot writes a slot entry at slotIdx directly into the page slice.
func WriteSlot(page []byte, slotIdx uint16, s Slot) {
	offset := PageHeaderSize + int(slotIdx)*SlotSize
	binary.LittleEndian.PutUint16(page[offset:offset+2], s.Offset)
	binary.LittleEndian.PutUint16(page[offset+2:offset+4], s.Length)
}
