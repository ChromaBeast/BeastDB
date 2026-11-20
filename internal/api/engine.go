package api

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

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
	failed         error          // set by halt; blocks writes until restart
	reclaim        map[uint64]int // data page -> free bytes after compaction, or unknownFree
	spaceMapPages  []uint64       // pages of the free-space map saved at the last checkpoint
	opts           Options
	mu             sync.RWMutex
	// imageMu serialises everything that writes the data file after open:
	// Checkpoint, Close and CreateSnapshot (which holds it while it copies the
	// file). Lock order: imageMu, then mu.
	imageMu      sync.Mutex
	snapshotHook func()          // tests only; see CreateSnapshot
	cq           committer       // group-commit queue (engine_commit.go)
	pending      []pendingNotify // commit events awaiting the group fsync; guarded by mu

	lastCkptLSN  atomic.Uint64 // LSN at the last successful checkpoint (metrics)
	lastCkptUnix atomic.Int64  // unix seconds of the last successful checkpoint

	// maxWALBytes is the auto-checkpoint threshold; adjustable at runtime.
	maxWALBytes atomic.Int64
}

// CurrentLSN returns the latest Log Sequence Number in the engine's WAL.
func (e *Engine) CurrentLSN() uint64 {
	return e.wal.CurrentLSN()
}

// DurableLSN returns the highest LSN known to be fsynced to the WAL.
func (e *Engine) DurableLSN() uint64 {
	return e.wal.DurableLSN()
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
		return e.commitReplicated(func() error { return e.putLocked(keyUint, value) })

	case wal.OpDelete:
		if len(key) < 8 {
			return nil
		}
		keyUint := binary.LittleEndian.Uint64(key)
		return e.commitReplicated(func() error { return e.deleteLocked(keyUint) })

	case wal.OpBatch:
		batchOps, err := wal.DecodeBatchPayload(value)
		if err != nil {
			return err
		}
		if len(batchOps) == 0 {
			return nil
		}
		return e.commit(func() error { return e.replicatedBatchLocked(value, batchOps) })

	case wal.OpEpoch:
		return e.commitReplicated(func() error {
			_, err := e.epochLocked(value)
			return err
		})

	default:
		return nil
	}
}

// replicatedBatchLocked logs a batch received from the leader in this node's
// WAL, exactly as the leader logged it, and then applies it. Logging keeps the
// batch across a crash and keeps this node's LSN in step with the leader's.
func (e *Engine) replicatedBatchLocked(payload []byte, ops []wal.BatchOp) error {
	if e.failed != nil {
		return e.failed
	}
	lsn, err := e.wal.Append(wal.OpBatch, nil, payload)
	if err != nil {
		return e.halt(err)
	}
	for _, op := range ops {
		if len(op.Key) < 8 {
			continue
		}
		k := binary.LittleEndian.Uint64(op.Key)
		switch op.Type {
		case wal.OpPut:
			if err := e.applyPutInMemory(k, op.Value); err != nil {
				return e.halt(err)
			}
			e.indexValueLocked(k, op.Value)
		case wal.OpDelete:
			if err := e.applyDeleteInMemory(k); err != nil {
				return e.halt(err)
			}
			e.unindexValueLocked(k)
		}
	}
	e.queueNotify(lsn, wal.OpBatch, nil, payload)
	return nil
}
