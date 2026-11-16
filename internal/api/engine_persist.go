package api

import (
	"encoding/binary"
	"errors"
	"log"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Chain kinds for structures the engine saves at a checkpoint.
const (
	chainSpaceMap  byte = 1
	chainIndexSnap byte = 2
)

// The free-space map (e.reclaim) is saved at every checkpoint so that space
// freed by deletes and updates is reused after a restart, not only until it.
// Entry: page ID (8) | free bytes (4, signed; unknownFree for "measure on use").
const spaceMapEntry = 12

// saveSpaceMap replaces the saved free-space map with the current one and
// returns its first page. Caller holds e.mu exclusively.
func (e *Engine) saveSpaceMap() (uint64, error) {
	storage.FreeChain(e.bpm, e.spaceMapPages)
	e.spaceMapPages = nil
	if len(e.reclaim) == 0 {
		return 0, nil
	}
	buf := make([]byte, 0, len(e.reclaim)*spaceMapEntry)
	for id, free := range e.reclaim {
		buf = binary.LittleEndian.AppendUint64(buf, id)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(int32(free)))
	}
	head, pages, err := storage.WriteChain(e.bpm, chainSpaceMap, buf)
	if err != nil {
		return 0, err
	}
	e.spaceMapPages = pages
	return head, nil
}

// loadSpaceMap restores the free-space map saved at the last checkpoint. A
// map that does not read back is dropped: it only holds hints, and a page it
// named is measured again before any insert uses it.
func (e *Engine) loadSpaceMap(head uint64) {
	if head == 0 {
		return
	}
	data, pages, err := storage.ReadChain(e.bpm, chainSpaceMap, head)
	if err != nil || len(data)%spaceMapEntry != 0 {
		log.Printf("engine: saved free-space map unusable, starting empty: %v", errOr(err, "bad length"))
		return
	}
	e.spaceMapPages = pages
	for off := 0; off < len(data); off += spaceMapEntry {
		id := binary.LittleEndian.Uint64(data[off:])
		free := int(int32(binary.LittleEndian.Uint32(data[off+8:])))
		if id == 0 || id >= e.disk.NumPages() {
			continue
		}
		e.trackFree(id, free)
	}
}

// saveIndexSnapshot writes the search index to new pages and returns the
// first. Caller holds e.mu exclusively.
func (e *Engine) saveIndexSnapshot() (uint64, error) {
	head, _, err := storage.WriteChain(e.bpm, chainIndexSnap, e.secIndex.Snapshot())
	return head, err
}

// loadIndexSnapshot restores the search index saved by a clean shutdown when
// it matches the checkpoint the engine opened at, and frees its pages either
// way: the index changes from here on, so the next checkpoint records none.
func (e *Engine) loadIndexSnapshot(meta storage.MetaData) (bool, error) {
	if meta.IndexSnapHead == 0 {
		return false, nil
	}
	data, pages, err := storage.ReadChain(e.bpm, chainIndexSnap, meta.IndexSnapHead)
	defer storage.FreeChain(e.bpm, pages)
	if err != nil {
		return false, err
	}
	if meta.IndexSnapLSN != meta.LastCheckpointLSN {
		return false, nil // saved before a later checkpoint; out of date
	}
	if err := e.secIndex.LoadSnapshot(data); err != nil {
		return false, err
	}
	return true, nil
}

func errOr(err error, msg string) error {
	if err != nil {
		return err
	}
	return errors.New(msg)
}
