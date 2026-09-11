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

	if disk.NumPages() == 0 {
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
		_ = bpm.UnpinPage(dataID, true)
		activeDataPage = dataID
	} else {
		tree = index.OpenBPlusTree(0, bpm)
		activeDataPage = 1
	}

	return &Engine{
		disk:           disk,
		bpm:            bpm,
		tree:           tree,
		wal:            w,
		activeDataPage: activeDataPage,
	}, nil
}

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

// Close flushes all dirty pages to physical disk and releases file handles.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_ = e.bpm.FlushAll()
	_ = e.wal.Close()
	return e.disk.Close()
}
