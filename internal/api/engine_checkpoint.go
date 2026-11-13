package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Close flushes all dirty pages to physical disk and releases file handles.
func (e *Engine) Close() error {
	e.stopCommitter() // drain queued writes before tearing down files
	e.mu.Lock()
	defer e.mu.Unlock()

	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	recordErr(e.persistCheckpointLocked(e.wal.CurrentLSN()))
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

	// Writes are applied in memory only inside a commit group that fsyncs before
	// releasing e.mu, so nothing un-synced can exist here; sync anyway so data
	// pages are never flushed ahead of their WAL records.
	if err := e.wal.SyncIfEnabled(); err != nil {
		return fmt.Errorf("checkpoint sync wal: %w", err)
	}

	// Dirty pages and the meta page must be durable before the WAL that covers
	// them is archived.
	ckptLSN := e.wal.CurrentLSN()
	if err := e.persistCheckpointLocked(ckptLSN); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	if err := e.wal.Checkpoint(); err != nil {
		return err
	}
	e.lastCkptLSN.Store(ckptLSN)
	e.lastCkptUnix.Store(time.Now().Unix())
	return nil
}

// persistCheckpointLocked records lsn as the recovery start point in the meta
// page and installs every page, meta included, in the data file as one atomic
// checkpoint. Recovery redoes the WAL after lsn on exactly this image.
// Caller holds e.mu exclusively.
func (e *Engine) persistCheckpointLocked(lsn uint64) error {
	if err := e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = lsn
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
	}); err != nil {
		return fmt.Errorf("update meta: %w", err)
	}
	if err := e.bpm.FlushAll(); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// StartAutoCheckpoint checkpoints whenever the WAL exceeds maxWALBytes, polling every
// interval, until ctx is cancelled. It bounds WAL size and crash-recovery time.
func (e *Engine) StartAutoCheckpoint(ctx context.Context, interval time.Duration, maxWALBytes int64) {
	e.maxWALBytes.Store(maxWALBytes)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if e.wal.FileSize() < e.maxWALBytes.Load() {
					continue
				}
				if err := e.Checkpoint(); err != nil {
					log.Printf("auto-checkpoint failed: %v", err)
				}
			}
		}
	}()
}

// SetCheckpointThreshold changes the WAL size at which the background
// checkpointer runs. It takes effect on the next tick; no restart is needed.
func (e *Engine) SetCheckpointThreshold(maxWALBytes int64) {
	if maxWALBytes > 0 {
		e.maxWALBytes.Store(maxWALBytes)
	}
}
