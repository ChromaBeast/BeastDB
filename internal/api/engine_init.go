package api

import (
	"fmt"
	"log"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// initFresh formats Page 0 as the meta page, creates the root B+ tree leaf,
// and the first data page.
func initFresh(disk *storage.DiskManager, bpm *storage.BufferPoolManager, w *wal.WAL, opts Options) (*Engine, error) {
	fail := func(err error) (*Engine, error) {
		_ = disk.Close()
		_ = w.Close()
		return nil, err
	}
	metaPage, metaID, err := bpm.NewPage()
	if err != nil {
		return fail(err)
	}
	tree, err := index.CreateBPlusTree(bpm)
	if err != nil {
		return fail(err)
	}
	_, dataID, err := bpm.NewPage()
	if err != nil {
		return fail(err)
	}

	meta := storage.MetaData{
		Magic:            storage.MetaMagic,
		Version:          storage.MetaVersion,
		RootPageID:       tree.RootPageID(),
		ActiveDataPageID: dataID,
	}
	if disk.Encrypted() {
		meta.Flags |= storage.MetaFlagEncrypted
	}
	storage.EncodeMeta(metaPage.Data(), meta)
	_ = bpm.UnpinPage(metaID, true)
	_ = bpm.UnpinPage(dataID, true)
	if err := bpm.FlushAll(); err != nil {
		return fail(err)
	}
	return newEngineInstance(disk, bpm, tree, w, meta, opts)
}

// newEngineInstance constructs the Engine, restores the free-space map and
// the search index, and redoes the WAL after the last checkpoint.
func newEngineInstance(disk *storage.DiskManager, bpm *storage.BufferPoolManager, tree *index.BPlusTree, w *wal.WAL, meta storage.MetaData, opts Options) (*Engine, error) {
	engine := &Engine{
		disk:           disk,
		bpm:            bpm,
		tree:           tree,
		wal:            w,
		activeDataPage: meta.ActiveDataPageID,
		secIndex:       index.NewSecondaryIndex(),
		idx:            newIndexRegistry(),
		opts:           opts,
	}
	// closeFiles releases the files without a checkpoint: after a failure part
	// way through opening, memory is not a state worth persisting.
	closeFiles := func(err error) (*Engine, error) {
		_ = w.Close()
		_ = disk.Close()
		return nil, err
	}

	tree.SetOnRootChange(func(newRootID uint64) {
		engine.updateMetaRoot(newRootID)
	})

	engine.loadSpaceMap(meta.SpaceMapHead)

	loaded, err := engine.loadIndexSnapshot(meta)
	if err != nil {
		log.Printf("engine: saved search index unusable, rebuilding: %v", err)
	}
	if !loaded {
		if err := engine.rebuildSecondaryIndex(); err != nil {
			return closeFiles(fmt.Errorf("rebuild secondary index: %w", err))
		}
	}

	if err := engine.openIndexes(); err != nil {
		return closeFiles(err)
	}
	if err := engine.recoverFromWAL(meta.LastCheckpointLSN); err != nil {
		return closeFiles(err)
	}
	engine.startIndexBuilds()
	return engine, nil
}
