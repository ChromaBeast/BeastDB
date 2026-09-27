package api

import (
	"encoding/binary"
	"errors"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
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
		return err
	}

	// 3. Apply each mutation to the buffer pool, B+ Tree, and secondary index
	for _, op := range ops {
		if op.Type == BatchOpPut {
			if err := e.applyPutInMemory(op.Key, op.Value); err != nil {
				return err
			}
			e.indexValueLocked(op.Key, op.Value)
		} else {
			if err := e.applyDeleteInMemory(op.Key); err != nil && err != index.ErrKeyNotFound {
				return err
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

func (e *Engine) applyPutInMemory(key uint64, value []byte) error {
	page, err := e.bpm.FetchPage(e.activeDataPage)
	if err != nil {
		return err
	}

	slotID, err := page.InsertTuple(value)
	if err == storage.ErrPageFull {
		_ = e.bpm.UnpinPage(e.activeDataPage, false)
		newPage, newID, err := e.bpm.NewPage()
		if err != nil {
			return err
		}
		if err := e.updateMetaActiveData(newID); err != nil {
			_ = e.bpm.UnpinPage(newID, false)
			return err
		}
		e.activeDataPage = newID
		slotID, err = newPage.InsertTuple(value)
		if err != nil {
			_ = e.bpm.UnpinPage(newID, false)
			return err
		}
		page = newPage
	}
	_ = e.bpm.UnpinPage(e.activeDataPage, true)

	rid := storage.RID{PageID: e.activeDataPage, SlotID: slotID}
	return e.tree.Insert(key, rid)
}

func (e *Engine) applyDeleteInMemory(key uint64) error {
	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		return nil
	}
	if err != nil {
		return err
	}

	page, err := e.bpm.FetchPage(rid.PageID)
	if err == nil {
		_ = page.DeleteTuple(rid.SlotID)
		_ = e.bpm.UnpinPage(rid.PageID, true)
	}

	return e.tree.Delete(key)
}
