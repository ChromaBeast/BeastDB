package api

import (
	"fmt"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// halt records an unrecoverable mid-apply failure. The WAL already holds the
// mutation, so memory and disk may disagree until the process restarts and
// replays it; all further writes are refused until then.
func (e *Engine) halt(err error) error {
	if e.failed == nil {
		e.failed = fmt.Errorf("engine halted, restart to replay WAL: %w", err)
	}
	return e.failed
}

// insertTuple stores value in a data page, reusing dead space before growing the file.
// Caller must hold e.mu exclusively.
func (e *Engine) insertTuple(value []byte) (storage.RID, error) {
	page, err := e.bpm.FetchPage(e.activeDataPage)
	if err != nil {
		return storage.RID{}, err
	}
	slotID, err := page.InsertTuple(value)
	if err == storage.ErrPageFull {
		// Tombstones on the active page may be holding reclaimable bytes.
		page.Compact()
		slotID, err = page.InsertTuple(value)
	}
	if err == nil {
		_ = e.bpm.UnpinPage(e.activeDataPage, true)
		return storage.RID{PageID: e.activeDataPage, SlotID: slotID}, nil
	}
	_ = e.bpm.UnpinPage(e.activeDataPage, true)
	if err != storage.ErrPageFull {
		return storage.RID{}, err
	}

	newPage, newID, err := e.nextDataPage(len(value))
	if err != nil {
		return storage.RID{}, err
	}
	if err := e.updateMetaActiveData(newID); err != nil {
		_ = e.bpm.UnpinPage(newID, false)
		return storage.RID{}, err
	}
	e.activeDataPage = newID
	slotID, err = newPage.InsertTuple(value)
	_ = e.bpm.UnpinPage(newID, err == nil)
	if err != nil {
		return storage.RID{}, err
	}
	return storage.RID{PageID: newID, SlotID: slotID}, nil
}

// nextDataPage returns a pinned page with room for size bytes: a compacted page
// with dead tuples if one fits, otherwise a freshly allocated page.
func (e *Engine) nextDataPage(size int) (*storage.SlottedPage, uint64, error) {
	for id := range e.reclaim {
		delete(e.reclaim, id)
		if id == e.activeDataPage {
			continue
		}
		p, err := e.bpm.FetchPage(id)
		if err != nil {
			continue
		}
		p.Compact()
		if p.FreeSpace() >= size+storage.SlotSize {
			_ = e.bpm.UnpinPage(id, true)
			p, err = e.bpm.FetchPage(id)
			if err == nil {
				return p, id, nil
			}
			continue
		}
		_ = e.bpm.UnpinPage(id, true)
	}
	return e.bpm.NewPage()
}

// freeTuple tombstones the tuple at rid and queues its page for space reuse.
func (e *Engine) freeTuple(rid storage.RID) {
	page, err := e.bpm.FetchPage(rid.PageID)
	if err != nil {
		return
	}
	_ = page.DeleteTuple(rid.SlotID)
	_ = e.bpm.UnpinPage(rid.PageID, true)
	if rid.PageID != e.activeDataPage {
		if e.reclaim == nil {
			e.reclaim = make(map[uint64]struct{})
		}
		e.reclaim[rid.PageID] = struct{}{}
	}
}

// applyPutInMemory writes value for key, freeing any tuple the key previously pointed at.
// It is idempotent per key, which is what makes WAL replay after a partial flush safe.
func (e *Engine) applyPutInMemory(key uint64, value []byte) error {
	oldRID, err := e.tree.Find(key)
	hadOld := err == nil
	if err != nil && err != index.ErrKeyNotFound {
		return err
	}
	rid, err := e.insertTuple(value)
	if err != nil {
		return err
	}
	if err := e.tree.Insert(key, rid); err != nil {
		return err
	}
	if hadOld {
		e.freeTuple(oldRID)
	}
	return nil
}

func (e *Engine) applyDeleteInMemory(key uint64) error {
	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	e.freeTuple(rid)
	return e.tree.Delete(key)
}
