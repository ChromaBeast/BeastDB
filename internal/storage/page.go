package storage

import (
	"errors"
)

var (
	ErrPageFull        = errors.New("page: insufficient free space")
	ErrSlotOutOfBounds = errors.New("page: slot index out of bounds")
	ErrTupleDeleted    = errors.New("page: tuple is deleted or tombstone")
	ErrInvalidPageSize = errors.New("page: data buffer must be exactly 4096 bytes")
	ErrCorruptPage     = errors.New("page: slot points outside the page")
)

// SlottedPage manages a single 4096-byte hardware-aligned disk block.
type SlottedPage struct {
	data []byte
	end  uint16 // where tuple data may end (PageDataEnd, or EncDataEnd when encrypted); 0 means PageDataEnd
}

func (p *SlottedPage) limit() int {
	if p.end == 0 {
		return PageDataEnd
	}
	return int(p.end)
}

// InitSlottedPage formats data in place as an empty slotted page.
func InitSlottedPage(data []byte, pageID uint64) *SlottedPage {
	return InitSlottedPageEnd(data, pageID, PageDataEnd)
}

// InitSlottedPageEnd is InitSlottedPage for a page whose bytes from end on are
// reserved (the encryption tag).
func InitSlottedPageEnd(data []byte, pageID uint64, end int) *SlottedPage {
	clear(data[:PageSize])
	WritePageHeader(data, PageHeader{PageID: pageID, Lower: PageHeaderSize, Upper: uint16(end)})
	return &SlottedPage{data: data[:PageSize], end: uint16(end)}
}

// LiveTuples returns the number of slots that hold a tuple.
func (p *SlottedPage) LiveTuples() int {
	h := p.Header()
	n := 0
	for i := uint16(0); i < h.SlotCount; i++ {
		if ReadSlot(p.data, i).Offset != 0 {
			n++
		}
	}
	return n
}

// NewSlottedPage initializes a fresh 4KB page with zero records and maximum free space.
func NewSlottedPage(pageID uint64) *SlottedPage { return NewSlottedPageEnd(pageID, PageDataEnd) }

// NewSlottedPageEnd is NewSlottedPage for a page whose last PageSize-end bytes
// are reserved (the checksum trailer, or the encryption tag).
func NewSlottedPageEnd(pageID uint64, end int) *SlottedPage {
	data := make([]byte, PageSize)
	header := PageHeader{
		PageID:    pageID,
		SlotCount: 0,
		Lower:     PageHeaderSize,
		Upper:     uint16(end),
	}
	WritePageHeader(data, header)
	return &SlottedPage{data: data, end: uint16(end)}
}

// WrapPage encapsulates an existing 4096-byte disk slice into a SlottedPage manager.
func WrapPage(data []byte) (*SlottedPage, error) { return WrapPageEnd(data, PageDataEnd) }

// WrapPageEnd is WrapPage for a page whose tail from end on is reserved.
func WrapPageEnd(data []byte, end int) (*SlottedPage, error) {
	if len(data) != PageSize {
		return nil, ErrInvalidPageSize
	}
	return &SlottedPage{data: data, end: uint16(end)}, nil
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
// A tombstoned slot is reused before the directory grows, so a page that is
// filled and emptied repeatedly keeps its full capacity.
func (p *SlottedPage) InsertTuple(tuple []byte) (uint16, error) {
	tupleLen := len(tuple)
	h := p.Header()
	slotID, reuse := p.freeSlot(h)
	needed := tupleLen
	if !reuse {
		needed += SlotSize
	}
	if p.FreeSpace() < needed {
		return 0, ErrPageFull
	}

	h.Upper -= uint16(tupleLen)
	copy(p.data[h.Upper:h.Upper+uint16(tupleLen)], tuple)
	WriteSlot(p.data, slotID, Slot{Offset: h.Upper, Length: uint16(tupleLen)})
	if !reuse {
		h.SlotCount++
		h.Lower += SlotSize
	}
	WritePageHeader(p.data, h)
	return slotID, nil
}

// freeSlot returns the first tombstoned slot, or the next new slot ID.
func (p *SlottedPage) freeSlot(h PageHeader) (uint16, bool) {
	for i := uint16(0); i < h.SlotCount; i++ {
		if ReadSlot(p.data, i).Offset == 0 {
			return i, true
		}
	}
	return h.SlotCount, false
}

// GetTuple retrieves the tuple bytes referenced by slotID.
func (p *SlottedPage) GetTuple(slotID uint16) ([]byte, error) {
	h := p.Header()
	if slotID >= h.SlotCount {
		return nil, ErrSlotOutOfBounds
	}

	if PageHeaderSize+(int(slotID)+1)*SlotSize > PageSize {
		return nil, ErrCorruptPage
	}
	slot := ReadSlot(p.data, slotID)
	if slot.Offset == 0 {
		return nil, ErrTupleDeleted
	}
	// A damaged page must surface as an error, not as a slice-bounds panic.
	if slot.Offset < PageHeaderSize || int(slot.Offset)+int(slot.Length) > PageSize {
		return nil, ErrCorruptPage
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

	// Live tuples are packed below the checksum trailer. A page written by a
	// version 1 file may hold more than that; it is then compacted against
	// the full page instead, which keeps every tuple.
	live := 0
	for i := uint16(0); i < h.SlotCount; i++ {
		live += int(ReadSlot(p.data, i).Length)
	}
	limit := p.limit()
	if p.end == PageDataEnd && int(h.Lower)+live > PageDataEnd {
		limit = PageSize // only a version 1 page can hold more
	}
	newUpper := uint16(limit)

	for i := uint16(0); i < h.SlotCount; i++ {
		slot := ReadSlot(p.data, i)
		if slot.Offset > 0 {
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
