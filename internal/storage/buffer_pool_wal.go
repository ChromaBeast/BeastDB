package storage

// SetWALBarrier installs the write-ahead barrier that FlushAll runs before a
// checkpoint writes pages to the data file. Call it before the pool is shared.
func (bpm *BufferPoolManager) SetWALBarrier(fn func() error) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()
	bpm.walBarrier = fn
}
