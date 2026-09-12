package api

import (
	"encoding/binary"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Put logs mutation to WAL, stores tuple in slotted page, and indexes key in B+ Tree.
func (e *Engine) Put(key uint64, value []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var keyBytes [8]byte
	binary.LittleEndian.PutUint64(keyBytes[:], key)
	if _, err := e.wal.Write(wal.OpPut, keyBytes[:], value); err != nil {
		return err
	}

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

// Get finds key in B+ Tree and reads tuple bytes from the slotted page.
func (e *Engine) Get(key uint64) ([]byte, bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	page, err := e.bpm.FetchPage(rid.PageID)
	if err != nil {
		return nil, false, err
	}

	tuple, err := page.GetTuple(rid.SlotID)
	_ = e.bpm.UnpinPage(rid.PageID, false)
	if err != nil {
		return nil, false, err
	}

	val := make([]byte, len(tuple))
	copy(val, tuple)
	return val, true, nil
}

// Delete marks tuple deleted in slotted page, removes index entry, and logs tombstone.
func (e *Engine) Delete(key uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var keyBytes [8]byte
	binary.LittleEndian.PutUint64(keyBytes[:], key)
	if _, err := e.wal.Write(wal.OpDelete, keyBytes[:], nil); err != nil {
		return err
	}

	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		return index.ErrKeyNotFound
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

// Scan returns a streaming cursor iterator over the requested key range.
func (e *Engine) Scan(startKey, endKey uint64) (*index.Cursor, error) {
	return e.tree.Scan(startKey, endKey)
}

// ReadTuple retrieves tuple bytes directly for a given RID.
func (e *Engine) ReadTuple(rid storage.RID) ([]byte, error) {
	page, err := e.bpm.FetchPage(rid.PageID)
	if err != nil {
		return nil, err
	}
	tuple, err := page.GetTuple(rid.SlotID)
	_ = e.bpm.UnpinPage(rid.PageID, false)
	if err != nil {
		return nil, err
	}

	val := make([]byte, len(tuple))
	copy(val, tuple)
	return val, nil
}
