package storage

import (
	"errors"
)

var (
	ErrPageFull         = errors.New("page: insufficient free space")
	ErrSlotOutOfBounds  = errors.New("page: slot index out of bounds")
	ErrTupleDeleted     = errors.New("page: tuple is deleted or tombstone")
	ErrInvalidPageSize  = errors.New("page: data buffer must be exactly 4096 bytes")
)

// SlottedPage manages a single 4096-byte hardware-aligned disk block.
type SlottedPage struct {
	data []byte
}

// NewSlottedPage initializes a fresh 4KB page with zero records and maximum free space.
func NewSlottedPage(pageID uint64) *SlottedPage {
	data := make([]byte, PageSize)
	header := PageHeader{
		PageID:    pageID,
		SlotCount: 0,
		Lower:     PageHeaderSize,
		Upper:     PageSize,
	}
	WritePageHeader(data, header)
	return &SlottedPage{data: data}
}

// WrapPage encapsulates an existing 4096-byte disk slice into a SlottedPage manager.
func WrapPage(data []byte) (*SlottedPage, error) {
	if len(data) != PageSize {
		return nil, ErrInvalidPageSize
	}
	return &SlottedPage{data: data}, nil
}

// Header returns the page's current metadata.
func (p *SlottedPage) Header() PageHeader {
	return ReadPageHeader(p.data)
}

// SetLSN updates the Log Sequence Number in the page header.
func (p *SlottedPage) SetLSN(lsn uint64) {
	h := p.Header()
	h.LSN = lsn
	WritePageHeader(p.data, h)
}

// FreeSpace calculates the contiguous unallocated bytes in the middle of the page.
func (p *SlottedPage) FreeSpace() int {
	h := p.Header()
	if h.Upper < h.Lower {
		return 0
	}
	return int(h.Upper - h.Lower)
}

// InsertTuple stores variable-length data, growing data upwards and slot downwards.
func (p *SlottedPage) InsertTuple(tuple []byte) (uint16, error) {
	tupleLen := len(tuple)
	needed := tupleLen + SlotSize

	if p.FreeSpace() < needed {
		return 0, ErrPageFull
	}

	h := p.Header()
	h.Upper -= uint16(tupleLen)
	copy(p.data[h.Upper:h.Upper+uint16(tupleLen)], tuple)

	slotID := h.SlotCount
	WriteSlot(p.data, slotID, Slot{
		Offset: h.Upper,
		Length: uint16(tupleLen),
	})

	h.SlotCount++
	h.Lower += SlotSize
	WritePageHeader(p.data, h)

	return slotID, nil
}

// GetTuple retrieves the tuple bytes referenced by slotID.
func (p *SlottedPage) GetTuple(slotID uint16) ([]byte, error) {
	h := p.Header()
	if slotID >= h.SlotCount {
		return nil, ErrSlotOutOfBounds
	}

	slot := ReadSlot(p.data, slotID)
	if slot.Offset == 0 {
		return nil, ErrTupleDeleted
	}

	return p.data[slot.Offset : slot.Offset+slot.Length], nil
}

// DeleteTuple marks the slot as a tombstone (Offset = 0) in O(1) time.
func (p *SlottedPage) DeleteTuple(slotID uint16) error {
	h := p.Header()
	if slotID >= h.SlotCount {
		return ErrSlotOutOfBounds
	}

	WriteSlot(p.data, slotID, Slot{Offset: 0, Length: 0})
	return nil
}

// Compact defragments the page by shifting active tuples tightly to the bottom.
func (p *SlottedPage) Compact() {
	h := p.Header()
	temp := make([]byte, PageSize)
	copy(temp[:PageHeaderSize], p.data[:PageHeaderSize])

	newUpper := uint16(PageSize)

	for i := uint16(0); i < h.SlotCount; i++ {
		slot := ReadSlot(p.data, i)
		if slot.Offset > 0 && slot.Length > 0 {
			newUpper -= slot.Length
			copy(temp[newUpper:newUpper+slot.Length], p.data[slot.Offset:slot.Offset+slot.Length])
			WriteSlot(temp, i, Slot{Offset: newUpper, Length: slot.Length})
		} else {
			WriteSlot(temp, i, Slot{Offset: 0, Length: 0})
		}
	}

	h.Upper = newUpper
	WritePageHeader(temp, h)
	copy(p.data, temp)
}

// Data returns the raw 4096-byte page slice for disk I/O.
func (p *SlottedPage) Data() []byte {
	return p.data
}
