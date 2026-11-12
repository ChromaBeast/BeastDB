package storage

// SetWALBarrier installs the write-ahead barrier. Call it before the pool is shared.
func (bpm *BufferPoolManager) SetWALBarrier(fn func() error) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()
	bpm.walBarrier = fn
}

// writeBack writes one dirty frame to the data file after the WAL barrier.
// Caller must hold bpm.mu (write lock).
func (bpm *BufferPoolManager) writeBack(f *Frame) error {
	if bpm.walBarrier != nil {
		if err := bpm.walBarrier(); err != nil {
			return err
		}
	}
	return bpm.disk.WritePage(f.PageID, f.Page.Data())
}
