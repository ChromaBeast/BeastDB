package api

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Engine coordinates durable storage, page management, indexing, and WAL recovery.
type Engine struct {
	disk           *storage.DiskManager
	bpm            *storage.BufferPoolManager
	tree           *index.BPlusTree
	wal            *wal.WAL
	activeDataPage uint64
	observer       CommitObserver
	secIndex       *index.SecondaryIndex
	mu             sync.RWMutex
}

// NewEngine initializes the database engine components with recovery support.
func NewEngine(dbPath, walPath string, poolSize int) (*Engine, error) {
	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		return nil, err
	}

	bpm := storage.NewBufferPoolManager(disk, poolSize)
	w, err := wal.OpenWAL(walPath, true)
	if err != nil {
		_ = disk.Close()
		return nil, err
	}

	if disk.NumPages() == 0 {
		return initFreshV1(disk, bpm, w)
	}

	metaPage, err := bpm.FetchPage(storage.MetaPageID)
	if err != nil {
		_ = disk.Close()
		_ = w.Close()
		return nil, err
	}

	data := metaPage.Data()

	if disk.NumPages() == 1 && isAllZeros(data) {
		_ = bpm.UnpinPage(storage.MetaPageID, false)
		_ = disk.Close()
		_ = w.Close()
		_ = os.Remove(dbPath)
		return NewEngine(dbPath, walPath, poolSize)
	}

	meta, err := storage.DecodeMeta(data)
	_ = bpm.UnpinPage(storage.MetaPageID, false)
	if err != nil {
		_ = disk.Close()
		_ = w.Close()

		if isLegacyV0Page(data) {
			if migErr := migrateV0ToV1(dbPath, walPath, poolSize); migErr != nil {
				return nil, fmt.Errorf("automatic migration from legacy format failed: %w", migErr)
			}
			return NewEngine(dbPath, walPath, poolSize)
		}

		return nil, err
	}

	tree := index.OpenBPlusTree(meta.RootPageID, bpm)
	return newEngineInstance(disk, bpm, tree, w, meta.ActiveDataPageID, meta.LastCheckpointLSN)
}

// CurrentLSN returns the latest Log Sequence Number in the engine's WAL.
func (e *Engine) CurrentLSN() uint64 {
	return e.wal.CurrentLSN()
}

// WALPath returns the filesystem location of the active WAL file.
func (e *Engine) WALPath() string {
	return e.wal.Path()
}

// ApplyReplicatedRecord writes a replicated mutation from Leader into local state.
func (e *Engine) ApplyReplicatedRecord(opType byte, key, value []byte) error {
	if len(key) < 8 {
		return nil
	}
	keyUint := binary.LittleEndian.Uint64(key)

	switch opType {
	case wal.OpPut:
		return e.Put(keyUint, value)
	case wal.OpDelete:
		_ = e.Delete(keyUint)
		return nil
	default:
		return nil
	}
}

// Close flushes all dirty pages to physical disk and releases file handles.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_ = e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = e.wal.CurrentLSN()
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
	})
	_ = e.bpm.FlushAll()
	_ = e.wal.Close()
	return e.disk.Close()
}

// Checkpoint flushes all dirty pages to disk, then rotates the WAL file.
// After a checkpoint, crash recovery only needs to scan the new (empty) WAL.
// Safe to call periodically (e.g., every N writes or on a timer).
func (e *Engine) Checkpoint() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_ = e.updateMeta(func(m *storage.MetaData) {
		m.LastCheckpointLSN = e.wal.CurrentLSN()
		m.ActiveDataPageID = e.activeDataPage
		m.RootPageID = e.tree.RootPageID()
	})
	if err := e.bpm.FlushAll(); err != nil {
		return err
	}
	return e.wal.Checkpoint()
}

