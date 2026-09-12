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
// mu is a RWMutex: cache hits use RLock (no disk I/O, only atomic pin ops).
// Cache misses and evictions use a full Lock.
type BufferPoolManager struct {
	disk      *DiskManager
	poolSize  int
	frames    []*Frame
	pageTable map[uint64]int
	clockHand int
	mu        sync.RWMutex
	pagePool  sync.Pool // Fix #6: reusable 4KB page buffers
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
		pagePool:  sync.Pool{New: func() any { buf := make([]byte, PageSize); return &buf }},
	}
}

// FetchPage retrieves a page from memory (fast RLock path) or loads from disk (write path).
func (bpm *BufferPoolManager) FetchPage(pageID uint64) (*SlottedPage, error) {
	// Fast path: cache hit — only atomic increments under shared lock.
	bpm.mu.RLock()
	if idx, exists := bpm.pageTable[pageID]; exists {
		frame := bpm.frames[idx]
		frame.Pin()
		page := frame.Page
		bpm.mu.RUnlock()
		return page, nil
	}
	bpm.mu.RUnlock()

	// Slow path: cache miss — need exclusive lock to evict + load.
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	// Double-check: another goroutine may have loaded the page while we waited.
	if idx, exists := bpm.pageTable[pageID]; exists {
		bpm.frames[idx].Pin()
		return bpm.frames[idx].Page, nil
	}

	return bpm.loadPage(pageID)
}

// NewPage allocates a new physical page on disk and loads it into a frame.
func (bpm *BufferPoolManager) NewPage() (*SlottedPage, uint64, error) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	victimIdx, err := bpm.findVictim()
	if err != nil {
		return nil, 0, err
	}
	if err := bpm.evictFrame(victimIdx); err != nil {
		return nil, 0, err
	}

	newPageID, err := bpm.disk.AllocatePage()
	if err != nil {
		return nil, 0, err
	}

	page := NewSlottedPage(newPageID)
	victim := bpm.frames[victimIdx]
	victim.PageID = newPageID
	victim.Pin()
	victim.IsDirty = true
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
	if frame.PinCount() <= 0 {
		return ErrPageNotPinned
	}
	if isDirty {
		frame.IsDirty = true
	}
	frame.Unpin()
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


