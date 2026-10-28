package api

import (
	"encoding/binary"
	"errors"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

var (
	// ErrEmptyBatchWrite indicates an attempt to execute an empty batch.
	ErrEmptyBatchWrite = errors.New("engine: batch write contains zero operations")
)

// BatchOpType specifies whether the batch operation is an insertion or deletion.
type BatchOpType byte

const (
	BatchOpPut    BatchOpType = 1
	BatchOpDelete BatchOpType = 2
)

// BatchOperation describes an individual mutation in an atomic batch.
type BatchOperation struct {
	Type  BatchOpType
	Key   uint64
	Value []byte
}

// BatchWrite executes an atomic batch of put/delete operations under exclusive engine locking.
// All operations are logged in a single WAL record with a single CRC32 checksum, ensuring
// that all operations commit together or none do.
func (e *Engine) BatchWrite(ops []BatchOperation) error {
	return e.BatchWriteIf(ops, nil)
}

// BatchWriteIf is BatchWrite gated by preconditions: every condition is evaluated
// under the same exclusive lock before any operation applies, so either all
// conditions hold and the whole batch commits, or ErrConditionFailed is returned
// and nothing is written.
func (e *Engine) BatchWriteIf(ops []BatchOperation, conds []KeyCondition) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	if len(ops) == 0 {
		return ErrEmptyBatchWrite
	}
	for _, op := range ops {
		if op.Type == BatchOpPut && len(op.Value) > MaxRecordSize {
			return ErrRecordTooLarge
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	for _, c := range conds {
		if err := e.checkConditionLocked(c.Key, c.Cond); err != nil {
			return err
		}
	}

	// 1. Serialize all operations into a single atomic WAL batch payload
	walOps := make([]wal.BatchOp, len(ops))
	for i, op := range ops {
		keyBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(keyBytes, op.Key)

		var opType byte
		if op.Type == BatchOpPut {
			opType = wal.OpPut
		} else {
			opType = wal.OpDelete
		}

		walOps[i] = wal.BatchOp{
			Type:  opType,
			Key:   keyBytes,
			Value: op.Value,
		}
	}

	payload, err := wal.EncodeBatchPayload(walOps)
	if err != nil {
		return err
	}

	// 2. Commit the entire batch atomically to the WAL
	lsn, err := e.wal.Write(wal.OpBatch, nil, payload)
	if err != nil {
		return e.halt(err)
	}

	// 3. Apply each mutation to the buffer pool, B+ Tree, and secondary index
	for _, op := range ops {
		if op.Type == BatchOpPut {
			if err := e.applyPutInMemory(op.Key, op.Value); err != nil {
				return e.halt(err)
			}
			e.indexValueLocked(op.Key, op.Value)
		} else {
			if err := e.applyDeleteInMemory(op.Key); err != nil && err != index.ErrKeyNotFound {
				return e.halt(err)
			}
			e.unindexValueLocked(op.Key)
		}
	}

	// 4. Notify replication observer of the atomic batch commit as a single unit
	if e.observer != nil {
		e.observer.OnCommit(lsn, wal.OpBatch, nil, payload)
	}

	return nil
}
