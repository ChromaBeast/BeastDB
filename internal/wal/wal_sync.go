package wal

func (w *WAL) loadSyncErr() error {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.syncErr
}

// WaitDurable blocks until the record with the given LSN has been fsynced.
// If another caller's fsync already covered it, no further fsync is issued.
func (w *WAL) WaitDurable(lsn uint64) error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	if err := w.loadSyncErr(); err != nil {
		return err
	}
	if w.syncedLSN >= lsn {
		return nil
	}
	return w.syncLocked()
}

// syncLocked fsyncs everything appended so far. Caller holds syncMu.
func (w *WAL) syncLocked() error {
	w.mu.Lock()
	target := w.currentLSN
	f := w.file
	w.mu.Unlock()

	// Appends may proceed while fsync runs; they simply miss this sync and are
	// covered by the next one. target only claims what was written before it.
	if err := f.Sync(); err != nil {
		w.errMu.Lock()
		w.syncErr = err
		w.errMu.Unlock()
		return err
	}
	if target > w.syncedLSN {
		w.syncedLSN = target
	}
	return nil
}

// Sync forces all buffered kernel writes to physical non-volatile storage.
func (w *WAL) Sync() error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	if err := w.loadSyncErr(); err != nil {
		return err
	}
	return w.syncLocked()
}

// SyncIfEnabled fsyncs only when the log was opened with syncOnWrite.
func (w *WAL) SyncIfEnabled() error {
	if !w.syncOnWrite {
		return nil
	}
	return w.Sync()
}

// EnsureDurable returns once every record appended so far is fsynced. It is a
// no-op when the log was not opened with syncOnWrite, and costs no fsync when
// a previous sync already covers the current LSN. The buffer pool calls it
// before writing a dirty page so a data page never reaches disk ahead of the
// WAL records that describe it.
func (w *WAL) EnsureDurable() error {
	if !w.syncOnWrite {
		return nil
	}
	return w.WaitDurable(w.CurrentLSN())
}

// DurableLSN returns the highest LSN known to be fsynced.
func (w *WAL) DurableLSN() uint64 {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	return w.syncedLSN
}
