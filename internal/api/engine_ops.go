package api

import (
	"encoding/binary"
	"errors"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

const MaxRecordSize = 4000

var (
	// ErrRecordExists indicates a key is already present during PutIfAbsent.
	ErrRecordExists = errors.New("record already exists")
	// ErrRecordTooLarge indicates the payload exceeds the 4,000 byte limit.
	ErrRecordTooLarge = errors.New("record value exceeds maximum supported size of 4000 bytes")
	// ErrReadOnlyReplica indicates mutation attempts on a read-only replica node.
	ErrReadOnlyReplica = errors.New("writes are not permitted on read-only replica")
)

// Put logs mutation to WAL, stores tuple in slotted page, and indexes key in B+ Tree.
func (e *Engine) Put(key uint64, value []byte) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	if len(value) > MaxRecordSize {
		return ErrRecordTooLarge
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.putLocked(key, value)
}

// PutIfAbsent inserts only when key is not already present.
func (e *Engine) PutIfAbsent(key uint64, value []byte) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	if len(value) > MaxRecordSize {
		return ErrRecordTooLarge
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.tree.Find(key)
	if err == nil {
		return ErrRecordExists
	}
	if err != index.ErrKeyNotFound {
		return err
	}
	return e.putLocked(key, value)
}

func (e *Engine) putLocked(key uint64, value []byte) error {
	var keyBytes [8]byte
	binary.LittleEndian.PutUint64(keyBytes[:], key)
	lsn, err := e.wal.Write(wal.OpPut, keyBytes[:], value)
	if err != nil {
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
	if err := e.tree.Insert(key, rid); err != nil {
		return err
	}

	e.indexValueLocked(key, value)
	e.notifyCommit(lsn, wal.OpPut, keyBytes[:], value)
	return nil
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


// Scan returns a streaming cursor iterator over the requested key range.
// The engine's read lock is held for the cursor's entire lifetime and released
// in cursor.Close() — this prevents concurrent writes from splitting a leaf
// that an active cursor is currently traversing.
func (e *Engine) Scan(startKey, endKey uint64) (*index.Cursor, error) {
	e.mu.RLock()
	cursor, err := e.tree.ScanWithRelease(startKey, endKey, e.mu.RUnlock)
	if err != nil {
		// ScanWithRelease calls release on error internally; nothing to do here.
		return nil, err
	}
	return cursor, nil
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
