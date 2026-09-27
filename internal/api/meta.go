package api

import (
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// readMeta loads and parses the MetaData stored in Page 0.
func (e *Engine) readMeta() (storage.MetaData, error) {
	page, err := e.bpm.FetchPage(storage.MetaPageID)
	if err != nil {
		return storage.MetaData{}, err
	}
	defer func() {
		_ = e.bpm.UnpinPage(storage.MetaPageID, false)
	}()

	return storage.DecodeMeta(page.Data())
}

// updateMeta modifies MetaData in Page 0 under buffer pool synchronization.
func (e *Engine) updateMeta(fn func(m *storage.MetaData)) error {
	page, err := e.bpm.FetchPage(storage.MetaPageID)
	if err != nil {
		return err
	}

	meta, err := storage.DecodeMeta(page.Data())
	if err != nil {
		_ = e.bpm.UnpinPage(storage.MetaPageID, false)
		return err
	}

	fn(&meta)
	storage.EncodeMeta(page.Data(), meta)
	return e.bpm.UnpinPage(storage.MetaPageID, true)
}

// updateMetaRoot persists a new B+ Tree root page ID to Page 0.
func (e *Engine) updateMetaRoot(newRootID uint64) {
	_ = e.updateMeta(func(m *storage.MetaData) {
		m.RootPageID = newRootID
	})
}

// updateMetaActiveData persists the active slotted data page ID to Page 0.
func (e *Engine) updateMetaActiveData(pageID uint64) {
	_ = e.updateMeta(func(m *storage.MetaData) {
		m.ActiveDataPageID = pageID
	})
}
