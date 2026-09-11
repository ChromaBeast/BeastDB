package storage

import (
	"errors"
	"sync"
)

var (
	ErrPageNotPinned = errors.New("buffer pool: unpinning unpinned page")
	ErrPageNotFound  = errors.New("buffer pool: page not found in pool")
	ErrPoolExhausted = errors.New("buffer pool: all frames currently pinned")
)

// BufferPoolManager coordinates page caching between memory frames and disk.
type BufferPoolManager struct {
	disk      *DiskManager
	poolSize  int
	frames    []*Frame
	pageTable map[uint64]int
	clockHand int
	mu        sync.Mutex
}

// NewBufferPoolManager creates a pool with a fixed number of in-memory frames.
func NewBufferPoolManager(disk *DiskManager, poolSize int) *BufferPoolManager {
	frames := make([]*Frame, poolSize)
	for i := range frames {
		frames[i] = &Frame{}
	}

	return &BufferPoolManager{
		disk:      disk,
		poolSize:  poolSize,
		frames:    frames,
		pageTable: make(map[uint64]int, poolSize),
	}
}

// FetchPage retrieves a page from memory or loads it from disk if not present.
func (bpm *BufferPoolManager) FetchPage(pageID uint64) (*SlottedPage, error) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	if idx, exists := bpm.pageTable[pageID]; exists {
		frame := bpm.frames[idx]
		frame.PinCount++
		frame.RefBit = true
		return frame.Page, nil
	}

	victimIdx, err := bpm.findVictim()
	if err != nil {
		return nil, err
	}

	victim := bpm.frames[victimIdx]
	if victim.Page != nil {
		if victim.IsDirty {
			if err := bpm.disk.WritePage(victim.PageID, victim.Page.Data()); err != nil {
				return nil, err
			}
			victim.IsDirty = false
		}
		delete(bpm.pageTable, victim.PageID)
	}

	pageData := make([]byte, PageSize)
	if err := bpm.disk.ReadPage(pageID, pageData); err != nil {
		return nil, err
	}

	page, err := WrapPage(pageData)
	if err != nil {
		return nil, err
	}

	victim.PageID = pageID
	victim.PinCount = 1
	victim.IsDirty = false
	victim.RefBit = true
	victim.Page = page

	bpm.pageTable[pageID] = victimIdx
	return page, nil
}

// NewPage allocates a new physical page on disk and loads it into a frame.
func (bpm *BufferPoolManager) NewPage() (*SlottedPage, uint64, error) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	victimIdx, err := bpm.findVictim()
	if err != nil {
		return nil, 0, err
	}

	newPageID, err := bpm.disk.AllocatePage()
	if err != nil {
		return nil, 0, err
	}

	victim := bpm.frames[victimIdx]
	if victim.Page != nil {
		if victim.IsDirty {
			if err := bpm.disk.WritePage(victim.PageID, victim.Page.Data()); err != nil {
				return nil, 0, err
			}
			victim.IsDirty = false
		}
		delete(bpm.pageTable, victim.PageID)
	}

	page := NewSlottedPage(newPageID)
	victim.PageID = newPageID
	victim.PinCount = 1
	victim.IsDirty = true
	victim.RefBit = true
	victim.Page = page

	bpm.pageTable[newPageID] = victimIdx
	return page, newPageID, nil
}

// UnpinPage decrements the frame pin count and marks dirty state.
func (bpm *BufferPoolManager) UnpinPage(pageID uint64, isDirty bool) error {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	idx, exists := bpm.pageTable[pageID]
	if !exists {
		return ErrPageNotFound
	}

	frame := bpm.frames[idx]
	if frame.PinCount <= 0 {
		return ErrPageNotPinned
	}

	if isDirty {
		frame.IsDirty = true
	}
	frame.PinCount--
	return nil
}

// FlushPage forces a cached page to disk if dirty.
func (bpm *BufferPoolManager) FlushPage(pageID uint64) error {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	idx, exists := bpm.pageTable[pageID]
	if !exists {
		return ErrPageNotFound
	}

	frame := bpm.frames[idx]
	if frame.Page != nil && frame.IsDirty {
		if err := bpm.disk.WritePage(frame.PageID, frame.Page.Data()); err != nil {
			return err
		}
		frame.IsDirty = false
	}
	return nil
}

// FlushAll flushes all dirty pages across all frames to persistent storage.
func (bpm *BufferPoolManager) FlushAll() error {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	for _, frame := range bpm.frames {
		if frame.Page != nil && frame.IsDirty {
			if err := bpm.disk.WritePage(frame.PageID, frame.Page.Data()); err != nil {
				return err
			}
			frame.IsDirty = false
		}
	}
	return bpm.disk.Sync()
}

// findVictim runs the Clock-Sweep eviction algorithm to select an unpinned frame.
func (bpm *BufferPoolManager) findVictim() (int, error) {
	for step := 0; step < 2*bpm.poolSize; step++ {
		idx := bpm.clockHand
		bpm.clockHand = (bpm.clockHand + 1) % bpm.poolSize

		frame := bpm.frames[idx]
		if frame.Page == nil {
			return idx, nil
		}

		if !frame.CanEvict() {
			continue
		}

		if frame.RefBit {
			frame.RefBit = false
		} else {
			return idx, nil
		}
	}
	return -1, ErrPoolExhausted
}
