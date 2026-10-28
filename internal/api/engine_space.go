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
	leftover := page.FreeSpace()
	_ = e.bpm.UnpinPage(e.activeDataPage, true)
	if err != storage.ErrPageFull {
		return storage.RID{}, err
	}
	oldActive := e.activeDataPage

	newPage, newID, err := e.nextDataPage(len(value))
	if err != nil {
		return storage.RID{}, err
	}
	if err := e.updateMetaActiveData(newID); err != nil {
		_ = e.bpm.UnpinPage(newID, false)
		return storage.RID{}, err
	}
	e.activeDataPage = newID
	if leftover >= minTrackedFree {
		e.trackFree(oldActive, leftover)
	}
	slotID, err = newPage.InsertTuple(value)
	_ = e.bpm.UnpinPage(newID, err == nil)
	if err != nil {
		return storage.RID{}, err
	}
	return storage.RID{PageID: newID, SlotID: slotID}, nil
}

const (
	// unknownFree marks a page whose tuples were tombstoned since it was last measured.
	unknownFree = -1
	// minTrackedFree drops pages too full to be worth revisiting.
	minTrackedFree = 64
)

// nextDataPage returns a pinned page with room for size bytes: a known page
// with enough free space if one exists, otherwise a freshly allocated page.
// Pages that are too full for this request stay tracked for smaller ones.
func (e *Engine) nextDataPage(size int) (*storage.SlottedPage, uint64, error) {
	need := size + storage.SlotSize
	for id, free := range e.reclaim {
		if id == e.activeDataPage {
			delete(e.reclaim, id)
			continue
		}
		if free != unknownFree && free < need {
			continue
		}
		p, err := e.bpm.FetchPage(id)
		if err != nil {
			delete(e.reclaim, id)
			continue
		}
		p.Compact()
		got := p.FreeSpace()
		_ = e.bpm.UnpinPage(id, true)
		if got >= need {
			delete(e.reclaim, id)
			if p, err = e.bpm.FetchPage(id); err == nil {
				return p, id, nil
			}
			continue
		}
		if got < minTrackedFree {
			delete(e.reclaim, id)
		} else {
			e.reclaim[id] = got
		}
	}
	return e.bpm.NewPage()
}

// trackFree remembers a page's free space for later inserts.
func (e *Engine) trackFree(id uint64, free int) {
	if e.reclaim == nil {
		e.reclaim = make(map[uint64]int)
	}
	if cur, ok := e.reclaim[id]; free == unknownFree || !ok || cur != unknownFree {
		e.reclaim[id] = free
	}
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
		e.trackFree(rid.PageID, unknownFree)
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
	rid, err := e.insertValue(value)
	if err != nil {
		return err
	}
	if err := e.tree.Insert(key, rid); err != nil {
		return err
	}
	if hadOld {
		e.freeValue(oldRID)
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
	e.freeValue(rid)
	return e.tree.Delete(key)
}
