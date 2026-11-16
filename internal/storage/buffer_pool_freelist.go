package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
)

// Freed pages form a singly linked list threaded through the pages
// themselves: a free page holds freePageMagic and the ID of the next free
// page. The head lives in the buffer pool and is saved in the meta page at
// each checkpoint, so the list is part of the same atomic image as the pages
// that stopped referencing the freed ones. NewPage takes from the list before
// growing the file.
var freePageMagic = []byte("BDBFREE1")

var (
	// ErrFreeMetaPage is returned for an attempt to free page 0.
	ErrFreeMetaPage = errors.New("buffer pool: the meta page cannot be freed")
	// ErrPageInUse is returned when a page being freed is pinned elsewhere.
	ErrPageInUse = errors.New("buffer pool: page is pinned and cannot be freed")
	// ErrDoubleFree is returned when a page is already on the free list.
	ErrDoubleFree = errors.New("buffer pool: page is already free")
)

// SetFreeListHead installs the free list saved in the meta page. Call it
// before the pool is shared.
func (bpm *BufferPoolManager) SetFreeListHead(id uint64) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()
	bpm.freeHead = id
}

// FreeListHead returns the first free page, or 0 when the list is empty.
func (bpm *BufferPoolManager) FreeListHead() uint64 {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()
	return bpm.freeHead
}

// FreePage puts pageID on the free list. The caller must have unpinned it
// and removed every reference to it.
func (bpm *BufferPoolManager) FreePage(pageID uint64) error {
	if pageID == MetaPageID {
		return ErrFreeMetaPage
	}
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	frame, err := bpm.pinLocked(pageID)
	if err != nil {
		return err
	}
	if frame.PinCount() > 1 {
		frame.Unpin()
		return ErrPageInUse
	}
	data := frame.Page.Data()
	if bytes.Equal(data[:8], freePageMagic) {
		frame.Unpin()
		return ErrDoubleFree
	}
	clear(data[:PageSize])
	copy(data[:8], freePageMagic)
	binary.LittleEndian.PutUint64(data[8:16], bpm.freeHead)
	frame.IsDirty = true
	frame.Unpin()
	bpm.freeHead = pageID
	return nil
}

// popFreeLocked takes the first page off the free list and formats it as an
// empty slotted page, returned pinned. A list that does not check out (a page
// without the free marker, or a next pointer past the end of the file) is
// abandoned rather than trusted: leaking pages is safe, handing out a page
// that is still in use is not. Caller holds bpm.mu.
func (bpm *BufferPoolManager) popFreeLocked() (*SlottedPage, uint64, bool) {
	id := bpm.freeHead
	if id == 0 {
		return nil, 0, false
	}
	abandon := func(reason string) (*SlottedPage, uint64, bool) {
		log.Printf("buffer pool: free list abandoned at page %d: %s", id, reason)
		bpm.freeHead = 0
		return nil, 0, false
	}
	if id >= bpm.disk.NumPages() {
		return abandon("page is past the end of the file")
	}
	frame, err := bpm.pinLocked(id)
	if err != nil {
		return abandon(err.Error())
	}
	data := frame.Page.Data()
	if !bytes.Equal(data[:8], freePageMagic) || frame.PinCount() > 1 {
		frame.Unpin()
		return abandon("page is not marked free")
	}
	next := binary.LittleEndian.Uint64(data[8:16])
	if next >= bpm.disk.NumPages() {
		next = 0 // drop the rest of a damaged list
	}
	page := InitSlottedPage(data, id)
	frame.Page = page
	frame.IsDirty = true
	bpm.freeHead = next
	return page, id, true
}

// pinLocked pins pageID, loading it if it is not cached. Caller holds bpm.mu.
func (bpm *BufferPoolManager) pinLocked(pageID uint64) (*Frame, error) {
	if idx, ok := bpm.pageTable[pageID]; ok {
		f := bpm.frames[idx]
		f.Pin()
		return f, nil
	}
	if _, err := bpm.loadPage(pageID); err != nil {
		return nil, err
	}
	return bpm.frames[bpm.pageTable[pageID]], nil
}
