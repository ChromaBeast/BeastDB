package storage

// HitMiss returns cumulative FetchPage cache hits and misses.
func (bpm *BufferPoolManager) HitMiss() (hits, misses uint64) {
	return bpm.hits.Load(), bpm.misses.Load()
}

// Stats returns operational metrics for the buffer pool.
func (bpm *BufferPoolManager) Stats() (poolSize, cachedPages, pinnedPages, dirtyPages int) {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	poolSize = bpm.poolSize
	cachedPages = len(bpm.pageTable)
	for _, frame := range bpm.frames {
		if frame.PinCount() > 0 {
			pinnedPages++
		}
		if frame.IsDirty {
			dirtyPages++
		}
	}
	return
}
