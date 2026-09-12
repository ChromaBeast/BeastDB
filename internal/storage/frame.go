package storage

import "sync/atomic"

// Frame represents a slot in the buffer pool holding a cached 4KB SlottedPage.
// PinCount and RefBit use atomic operations so cache-hit FetchPage can proceed
// under the buffer pool's shared (read) lock without a global write lock.
type Frame struct {
	PageID   uint64
	pinCount atomic.Int32
	IsDirty  bool
	refBit   atomic.Bool
	Page     *SlottedPage
}

// PinCount returns the current pin count atomically.
func (f *Frame) PinCount() int32 {
	return f.pinCount.Load()
}

// Pin atomically increments the pin count and marks the frame recently used.
func (f *Frame) Pin() {
	f.pinCount.Add(1)
	f.refBit.Store(true)
}

// Unpin decrements the pin count; returns the new count.
func (f *Frame) Unpin() int32 {
	return f.pinCount.Add(-1)
}

// RefBit returns the reference bit atomically.
func (f *Frame) RefBit() bool {
	return f.refBit.Load()
}

// ClearRefBit atomically clears the reference bit (called by clock-sweep).
func (f *Frame) ClearRefBit() {
	f.refBit.Store(false)
}

// Reset clears the frame state for recycling by another page.
func (f *Frame) Reset() {
	f.PageID = 0
	f.pinCount.Store(0)
	f.IsDirty = false
	f.refBit.Store(false)
	f.Page = nil
}

// CanEvict returns true if the frame is unpinned and eligible for eviction.
func (f *Frame) CanEvict() bool {
	return f.pinCount.Load() == 0
}
