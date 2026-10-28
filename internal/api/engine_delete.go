package api

import (
	"encoding/binary"

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
	if e.failed != nil {
		return e.failed
	}
	var keyBytes [8]byte
	binary.LittleEndian.PutUint64(keyBytes[:], key)
	lsn, err := e.wal.Write(wal.OpDelete, keyBytes[:], nil)
	if err != nil {
		return e.halt(err)
	}

	if err := e.applyDeleteInMemory(key); err != nil {
		return e.halt(err)
	}

	e.unindexValueLocked(key)
	e.notifyCommit(lsn, wal.OpDelete, keyBytes[:], nil)
	return nil
}
