package storage

// loadPage selects a victim frame, evicts if needed, reads pageID from disk.
// Caller must hold bpm.mu (write lock).
func (bpm *BufferPoolManager) loadPage(pageID uint64) (*SlottedPage, error) {
	victimIdx, err := bpm.findVictim()
	if err != nil {
		return nil, err
	}
	if err := bpm.evictFrame(victimIdx); err != nil {
		return nil, err
	}

	// Borrow a buffer from the pool — avoids a 4KB heap allocation per cache miss.
	bufPtr := bpm.pagePool.Get().(*[]byte)
	pageData := *bufPtr
	clear(pageData)

	if err := bpm.disk.ReadPage(pageID, pageData); err != nil {
		bpm.pagePool.Put(bufPtr)
		return nil, err
	}

	page, err := WrapPage(pageData)
	if err != nil {
		bpm.pagePool.Put(bufPtr)
		return nil, err
	}

	victim := bpm.frames[victimIdx]
	victim.PageID = pageID
	victim.Pin()
	victim.IsDirty = false
	victim.Page = page

	bpm.pageTable[pageID] = victimIdx
	return page, nil
}

// evictFrame flushes (if dirty) and clears the given frame for reuse.
// Caller must hold bpm.mu (write lock).
func (bpm *BufferPoolManager) evictFrame(idx int) error {
	victim := bpm.frames[idx]
	if victim.Page == nil {
		return nil
	}
	if victim.IsDirty {
		if err := bpm.disk.WritePage(victim.PageID, victim.Page.Data()); err != nil {
			return err
		}
		victim.IsDirty = false
	}
	delete(bpm.pageTable, victim.PageID)
	victim.Reset()
	return nil
}

// findVictim runs the Clock-Sweep eviction algorithm to select an unpinned frame.
// Caller must hold bpm.mu (write lock).
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
		if frame.RefBit() {
			frame.ClearRefBit()
		} else {
			return idx, nil
		}
	}
	return -1, ErrPoolExhausted
}
