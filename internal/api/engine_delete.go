package api

import (
	"encoding/binary"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Delete marks tuple deleted in slotted page, removes index entry, and logs tombstone.
func (e *Engine) Delete(key uint64) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.deleteLocked(key)
}

func (e *Engine) deleteLocked(key uint64) error {
	var keyBytes [8]byte
	binary.LittleEndian.PutUint64(keyBytes[:], key)
	lsn, err := e.wal.Write(wal.OpDelete, keyBytes[:], nil)
	if err != nil {
		return err
	}

	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		e.notifyCommit(lsn, wal.OpDelete, keyBytes[:], nil)
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

	if err := e.tree.Delete(key); err != nil {
		return err
	}

	e.unindexValueLocked(key)
	e.notifyCommit(lsn, wal.OpDelete, keyBytes[:], nil)
	return nil
}
