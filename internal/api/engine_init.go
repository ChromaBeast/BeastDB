package api

import (
	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// initFreshV1 formats Page 0 as MetaPage, creates root B+ tree, and initializes active data page.
func initFreshV1(disk *storage.DiskManager, bpm *storage.BufferPoolManager, w *wal.WAL) (*Engine, error) {
	metaPage, metaID, err := bpm.NewPage()
	if err != nil {
		_ = disk.Close()
		_ = w.Close()
		return nil, err
	}

	tree, err := index.CreateBPlusTree(bpm)
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

	storage.EncodeMeta(metaPage.Data(), storage.MetaData{
		Magic:             storage.MetaMagic,
		Version:           storage.MetaVersion,
		RootPageID:        tree.RootPageID(),
		ActiveDataPageID:  dataID,
		LastCheckpointLSN: 0,
	})
	_ = bpm.UnpinPage(metaID, true)
	_ = bpm.UnpinPage(dataID, true)
	if err := bpm.FlushAll(); err != nil {
		_ = disk.Close()
		_ = w.Close()
		return nil, err
	}

	return newEngineInstance(disk, bpm, tree, w, dataID, 0)
}

// newEngineInstance constructs the Engine struct, binds observers, and recovers from WAL.
func newEngineInstance(disk *storage.DiskManager, bpm *storage.BufferPoolManager, tree *index.BPlusTree, w *wal.WAL, activeDataPage, lastCheckpointLSN uint64) (*Engine, error) {
	engine := &Engine{
		disk:           disk,
		bpm:            bpm,
		tree:           tree,
		wal:            w,
		activeDataPage: activeDataPage,
		secIndex:       index.NewSecondaryIndex(),
	}

	tree.SetOnRootChange(func(newRootID uint64) {
		engine.updateMetaRoot(newRootID)
	})

	if disk.NumPages() > 0 {
		_ = engine.rebuildSecondaryIndex()
	}

	if err := engine.recoverFromWAL(lastCheckpointLSN); err != nil {
		_ = engine.Close()
		return nil, err
	}

	return engine, nil
}
