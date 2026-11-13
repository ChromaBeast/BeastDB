package storage

// SpillPage stores an evicted dirty page in the spill file. It is not durable
// and never touches the data file, so it needs neither an fsync nor the WAL
// barrier: after a crash the spill file is discarded and the WAL redoes the
// change from the last checkpoint image.
func (d *DiskManager) SpillPage(pageID uint64, pageData []byte) error {
	if len(pageData) < PageSize {
		return ErrDiskBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	slot, ok := d.spillSlots[pageID]
	if !ok {
		slot = d.spillNext
		d.spillNext++
		d.spillSlots[pageID] = slot
	}
	_, err := d.spill.WriteAt(pageData[:PageSize], slot*PageSize)
	return err
}

// resetSpillLocked empties the spill area once a checkpoint has made every
// spilled page part of the data file. Caller holds d.mu.
func (d *DiskManager) resetSpillLocked() error {
	clear(d.spillSlots)
	d.spillNext = 0
	return d.spill.Truncate(0)
}
