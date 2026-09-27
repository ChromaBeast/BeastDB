package api

import (
	"fmt"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Close flushes all dirty pages to physical disk and releases file handles.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	recordErr(e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = e.wal.CurrentLSN()
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
	}))
	recordErr(e.bpm.FlushAll())
	recordErr(e.wal.Close())
	recordErr(e.disk.Close())
	return firstErr
}

// Checkpoint flushes all dirty pages to disk, then rotates the WAL file.
// After a checkpoint, crash recovery only needs to scan the new (empty) WAL.
// Safe to call periodically (e.g., every N writes or on a timer).
func (e *Engine) Checkpoint() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = e.wal.CurrentLSN()
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
	}); err != nil {
		return fmt.Errorf("checkpoint update meta: %w", err)
	}
	if err := e.bpm.FlushAll(); err != nil {
		return fmt.Errorf("checkpoint flush dirty pages: %w", err)
	}
	return e.wal.Checkpoint()
}
