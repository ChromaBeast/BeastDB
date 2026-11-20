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
	e.stopIndexBuilds()
	e.mu.Lock()
	defer e.mu.Unlock()

	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if e.failed != nil {
		// Memory may hold a half-applied write. Leave the last checkpoint image
		// in place so the next open redoes the WAL from it.
		recordErr(e.failed)
	} else {
		// A clean shutdown also saves the search index, so the next open does
		// not have to rebuild it by scanning every record.
		recordErr(e.persistCheckpointWithIndexLocked(e.wal.CurrentLSN(), true))
	}
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
	if e.failed != nil {
		return e.failed // never persist a halted engine's in-memory state
	}

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
	return e.persistCheckpointWithIndexLocked(lsn, false)
}

// persistCheckpointWithIndexLocked is persistCheckpointLocked that can also
// save the search index. Only Close saves it: it is written in full, which is
// too costly for every periodic checkpoint.
func (e *Engine) persistCheckpointWithIndexLocked(lsn uint64, saveIndex bool) error {
	spaceHead, err := e.saveSpaceMap()
	if err != nil {
		return fmt.Errorf("save free-space map: %w", err)
	}
	var indexHead uint64
	if saveIndex {
		if indexHead, err = e.saveIndexSnapshot(); err != nil {
			return fmt.Errorf("save search index: %w", err)
		}
	}
	if err := e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = lsn
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
		m.SpaceMapHead = spaceHead
		m.IndexSnapHead = indexHead
		m.IndexSnapLSN = lsn
		// Read last: saving the maps above takes and returns free pages.
		m.FreeListHead = e.bpm.FreeListHead()
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
