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
	readOnly       bool
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

// SetReadOnly toggles read-only mode for replica nodes.
func (e *Engine) SetReadOnly(ro bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.readOnly = ro
}

// IsReadOnly reports whether the engine is currently in read-only mode.
func (e *Engine) IsReadOnly() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.readOnly
}

// ApplyReplicatedRecord writes a replicated mutation from Leader into local state.
func (e *Engine) ApplyReplicatedRecord(opType byte, key, value []byte) error {
	switch opType {
	case wal.OpPut:
		if len(key) < 8 {
			return nil
		}
		keyUint := binary.LittleEndian.Uint64(key)
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.putLocked(keyUint, value)

	case wal.OpDelete:
		if len(key) < 8 {
			return nil
		}
		keyUint := binary.LittleEndian.Uint64(key)
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.deleteLocked(keyUint)

	case wal.OpBatch:
		batchOps, err := wal.DecodeBatchPayload(value)
		if err != nil {
			return err
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		for _, bOp := range batchOps {
			if len(bOp.Key) < 8 {
				continue
			}
			k := binary.LittleEndian.Uint64(bOp.Key)
			if bOp.Type == wal.OpPut {
				if err := e.applyPutInMemory(k, bOp.Value); err != nil {
					return err
				}
				e.indexValueLocked(k, bOp.Value)
			} else if bOp.Type == wal.OpDelete {
				if err := e.applyDeleteInMemory(k); err != nil {
					return err
				}
				e.unindexValueLocked(k)
			}
		}
		return nil

	default:
		return nil
	}
}

