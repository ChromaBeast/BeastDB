package api

import (
	"encoding/binary"
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

	var tree *index.BPlusTree
	var activeDataPage uint64
	var lastCheckpointLSN uint64

	if disk.NumPages() == 0 {
		metaPage, metaID, err := bpm.NewPage()
		if err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}

		tree, err = index.CreateBPlusTree(bpm)
		if err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}

		_, dataID, err := bpm.NewPage()
		if err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}

		storage.EncodeMeta(metaPage.Data(), storage.MetaData{
			Magic:             storage.MetaMagic,
			Version:           storage.MetaVersion,
			RootPageID:        tree.RootPageID(),
			ActiveDataPageID:  dataID,
			LastCheckpointLSN: 0,
		})
		_ = bpm.UnpinPage(metaID, true)
		_ = bpm.UnpinPage(dataID, true)
		if err := bpm.FlushAll(); err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}
		activeDataPage = dataID
	} else {
		metaPage, err := bpm.FetchPage(storage.MetaPageID)
		if err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}
		meta, err := storage.DecodeMeta(metaPage.Data())
		_ = bpm.UnpinPage(storage.MetaPageID, false)
		if err != nil {
			_ = disk.Close()
			_ = w.Close()
			return nil, err
		}

		tree = index.OpenBPlusTree(meta.RootPageID, bpm)
		activeDataPage = meta.ActiveDataPageID
		lastCheckpointLSN = meta.LastCheckpointLSN
	}

	engine := &Engine{
		disk:           disk,
		bpm:            bpm,
		tree:           tree,
		wal:            w,
		activeDataPage: activeDataPage,
		secIndex:       index.NewSecondaryIndex(),
	}

	tree.SetOnRootChange(func(newRootID uint64) {
		engine.updateMetaRoot(newRootID)
	})

	if disk.NumPages() > 0 {
		_ = engine.rebuildSecondaryIndex()
	}

	if err := engine.recoverFromWAL(lastCheckpointLSN); err != nil {
		_ = engine.Close()
		return nil, err
	}

	return engine, nil
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

