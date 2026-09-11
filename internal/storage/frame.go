package storage

// Frame represents a slot in the buffer pool holding a cached 4KB SlottedPage.
type Frame struct {
	PageID   uint64
	PinCount int32
	IsDirty  bool
	RefBit   bool
	Page     *SlottedPage
}

// Reset clears the frame state for recycling by another page.
func (f *Frame) Reset() {
	f.PageID = 0
	f.PinCount = 0
	f.IsDirty = false
	f.RefBit = false
	f.Page = nil
}

// CanEvict returns true if the frame is unpinned and eligible for eviction.
func (f *Frame) CanEvict() bool {
	return f.PinCount == 0
}
