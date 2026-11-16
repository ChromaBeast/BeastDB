package api

import (
	"errors"
	"fmt"
	"os"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Options tunes how an engine opens its files.
type Options struct {
	// TruncateCorruptWAL lets the engine open a WAL that is damaged in the
	// middle (valid records follow the damage). Everything from the damage on
	// is saved to <wal>.discarded and dropped from the log, so writes that
	// were acknowledged may be lost. Without it such a WAL fails the open
	// with wal.ErrCorruptLog.
	TruncateCorruptWAL bool
}

// ErrMetaPageDamaged is returned when page 0 is blank but the database holds
// data. The file is left untouched for repair or restore.
var ErrMetaPageDamaged = errors.New("engine: meta page is blank but the database is not empty; refusing to reinitialize")

// NewEngine initializes the database engine components with recovery support.
func NewEngine(dbPath, walPath string, poolSize int) (*Engine, error) {
	return NewEngineWithOptions(dbPath, walPath, poolSize, Options{})
}

// NewEngineWithOptions is NewEngine with open-time options.
func NewEngineWithOptions(dbPath, walPath string, poolSize int, opts Options) (*Engine, error) {
	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		return nil, err
	}
	return openEngine(disk, dbPath, walPath, poolSize, true, opts)
}

// openEngine builds the engine on an open data file. dbPath is used only to
// recreate or migrate the file; crash tests pass a simulated file. syncWAL is
// always true in production; crash tests that never lose WAL writes turn it
// off so they do not fsync on every commit.
func openEngine(disk *storage.DiskManager, dbPath, walPath string, poolSize int, syncWAL bool, opts Options) (*Engine, error) {
	bpm := storage.NewBufferPoolManager(disk, poolSize)
	w, err := wal.OpenWALWithOptions(walPath, syncWAL, wal.Options{TruncateCorrupt: opts.TruncateCorruptWAL})
	if err != nil {
		_ = disk.Close()
		return nil, err
	}
	// Write-ahead barrier: no dirty data page may reach the data file before
	// the WAL records that describe it are durable (group commit applies pages
	// before the group's fsync).
	bpm.SetWALBarrier(w.EnsureDurable)

	if disk.NumPages() == 0 {
		return initFresh(disk, bpm, w, opts)
	}

	closeAll := func() {
		_ = disk.Close()
		_ = w.Close()
	}
	reopen := func() (*Engine, error) { return NewEngineWithOptions(dbPath, walPath, poolSize, opts) }

	metaPage, err := bpm.FetchPage(storage.MetaPageID)
	if err != nil {
		closeAll()
		return nil, err
	}
	data := metaPage.Data()

	// A zero meta page on a file that is zero throughout, with no WAL history,
	// is a database whose initialization never completed: start over. A zero
	// meta page in front of real data or WAL records is damage, and recreating
	// the file would destroy what is left, so refuse to open instead.
	if isAllZeros(data) {
		_ = bpm.UnpinPage(storage.MetaPageID, false)
		empty, zerr := dataFileIsZero(disk)
		hasWAL := w.CurrentLSN() != 0
		closeAll()
		if zerr != nil {
			return nil, zerr
		}
		if !empty || hasWAL {
			return nil, ErrMetaPageDamaged
		}
		_ = os.Remove(dbPath)
		return reopen()
	}

	meta, err := storage.DecodeMeta(data)
	_ = bpm.UnpinPage(storage.MetaPageID, false)
	if err != nil {
		legacyV0 := isLegacyV0Page(data)
		v, hasMagic := storage.MetaVersionOf(data)
		checksums := disk.Checksums()
		closeAll()

		switch {
		case hasMagic && v == 1 && !checksums:
			if migErr := migrateV1ToV2(dbPath, walPath, poolSize); migErr != nil {
				return nil, fmt.Errorf("automatic migration from format version 1 failed: %w", migErr)
			}
			return reopen()
		case !hasMagic && legacyV0 && !checksums:
			if migErr := migrateV0(dbPath, walPath, poolSize); migErr != nil {
				return nil, fmt.Errorf("automatic migration from legacy format failed: %w", migErr)
			}
			return reopen()
		}
		return nil, err
	}
	if !disk.Checksums() {
		closeAll()
		return nil, fmt.Errorf("engine: format version %d meta page in a file without page checksums", meta.Version)
	}

	w.SetCurrentLSN(meta.LastCheckpointLSN)
	bpm.SetFreeListHead(meta.FreeListHead)
	tree := index.OpenBPlusTree(meta.RootPageID, bpm)
	return newEngineInstance(disk, bpm, tree, w, meta, opts)
}

// dataFileIsZero reports whether every page of the data file is zero-filled.
func dataFileIsZero(disk *storage.DiskManager) (bool, error) {
	buf := make([]byte, storage.PageSize)
	for id := uint64(0); id < disk.NumPages(); id++ {
		clear(buf)
		if err := disk.ReadPage(id, buf); err != nil {
			return false, err
		}
		if !isAllZeros(buf) {
			return false, nil
		}
	}
	return true, nil
}
