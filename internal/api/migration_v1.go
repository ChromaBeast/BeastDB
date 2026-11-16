package api

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Batches written while copying records into the new file.
const (
	migrateBatchOps   = 1024
	migrateBatchBytes = 4 << 20
)

// migrateV1ToV2 rewrites a format version 1 database (no page checksums) as
// version 2. It copies every record of the last checkpoint image, then redoes
// the WAL records after that checkpoint, into a new file, and only then swaps
// the files. The old files are kept as *.v1.bak.
//
// The new database continues the old LSN sequence, so a follower that was
// streaming from this node sees a gap and fails loudly instead of skipping.
func migrateV1ToV2(dbPath, walPath string, poolSize int) error {
	log.Printf("Detected BeastDB format version 1 at %q. Migrating to version %d (page checksums)...", dbPath, storage.MetaVersion)

	src, err := openV1Reader(dbPath, poolSize)
	if err != nil {
		return err
	}
	// The old data file must be closed before installMigratedFiles renames
	// the new one over it: Windows refuses to replace an open file.
	srcOpen := true
	closeSrc := func() error {
		if !srcOpen {
			return nil
		}
		srcOpen = false
		return src.disk.Close()
	}
	defer closeSrc()

	tmpDB, tmpWAL := dbPath+".migrated.tmp", walPath+".migrated.tmp"
	removeMigrationTemps(tmpDB, tmpWAL)
	disk, err := storage.OpenDiskManager(tmpDB)
	if err != nil {
		return err
	}
	// The temporary WAL is not fsynced per write: if the process dies, the
	// temporary files are discarded and the migration runs again.
	dst, err := openEngine(disk, tmpDB, tmpWAL, poolSize, false, Options{})
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = dst.Close()
		removeMigrationTemps(tmpDB, tmpWAL)
		return err
	}
	dst.wal.SetCurrentLSN(highestOldLSN(walPath, src.meta.LastCheckpointLSN))

	copied, err := copyRecords(src.engine, dst)
	if err != nil {
		return fail(fmt.Errorf("copy records: %w", err))
	}
	redone, err := redoWALTail(walPath, src.meta.LastCheckpointLSN, dst)
	if err != nil {
		return fail(fmt.Errorf("redo WAL: %w", err))
	}
	if err := dst.Close(); err != nil {
		removeMigrationTemps(tmpDB, tmpWAL)
		return fmt.Errorf("close migrated database: %w", err)
	}
	if err := closeSrc(); err != nil {
		removeMigrationTemps(tmpDB, tmpWAL)
		return fmt.Errorf("close version 1 database: %w", err)
	}
	if err := installMigratedFiles(dbPath, walPath, tmpDB, tmpWAL, ".v1.bak"); err != nil {
		return err
	}
	log.Printf("Migrated %d records (%d redone from the WAL) to format version %d. Old files kept as *.v1.bak.", copied, redone, storage.MetaVersion)
	return nil
}

type v1Reader struct {
	disk   *storage.DiskManager
	engine *Engine // read-only shell: tree and value reads, no WAL
	meta   storage.MetaData
}

// openV1Reader opens a version 1 file for reading its checkpoint image.
func openV1Reader(dbPath string, poolSize int) (*v1Reader, error) {
	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		return nil, err
	}
	if disk.Checksums() {
		_ = disk.Close()
		return nil, fmt.Errorf("%s is not a version 1 file", dbPath)
	}
	bpm := storage.NewBufferPoolManager(disk, poolSize)
	page := make([]byte, storage.PageSize)
	if err := disk.ReadPage(storage.MetaPageID, page); err != nil {
		_ = disk.Close()
		return nil, err
	}
	meta, err := storage.DecodeMetaV1(page)
	if err != nil {
		_ = disk.Close()
		return nil, err
	}
	tree := index.OpenBPlusTree(meta.RootPageID, bpm)
	return &v1Reader{disk: disk, meta: meta, engine: &Engine{disk: disk, bpm: bpm, tree: tree}}, nil
}

// copyRecords writes every record of src into dst in batches.
func copyRecords(src, dst *Engine) (int, error) {
	cursor, err := src.tree.Scan(0, math.MaxUint64)
	if err != nil {
		return 0, err
	}
	defer cursor.Close()

	var batch []BatchOperation
	size, n := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := dst.BatchWrite(batch)
		batch, size = batch[:0], 0
		return err
	}
	for {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			return n, err
		}
		if !ok {
			return n, flush()
		}
		val, err := src.readValue(rid)
		if err != nil {
			return n, fmt.Errorf("read key %d: %w", key, err)
		}
		batch = append(batch, BatchOperation{Type: BatchOpPut, Key: key, Value: val})
		size += len(val) + walBatchOpOverhead
		n++
		if len(batch) >= migrateBatchOps || size >= migrateBatchBytes {
			if err := flush(); err != nil {
				return n, err
			}
		}
	}
}

// redoWALTail applies the WAL records after the old checkpoint to dst.
func redoWALTail(walPath string, ckptLSN uint64, dst *Engine) (int, error) {
	n := 0
	_, err := wal.Replay(walPath, func(rec *wal.Record) error {
		if rec.LSN <= ckptLSN {
			return nil
		}
		n++
		switch rec.Type {
		case wal.OpPut, wal.OpDelete:
			if len(rec.Key) < 8 {
				return nil
			}
			k := binary.LittleEndian.Uint64(rec.Key)
			if rec.Type == wal.OpPut {
				return dst.Put(k, rec.Value)
			}
			return dst.Delete(k)
		case wal.OpBatch:
			ops, err := wal.DecodeBatchPayload(rec.Value)
			if err != nil {
				return err
			}
			batch := make([]BatchOperation, 0, len(ops))
			for _, op := range ops {
				if len(op.Key) < 8 {
					continue
				}
				t := BatchOpPut
				if op.Type == wal.OpDelete {
					t = BatchOpDelete
				}
				batch = append(batch, BatchOperation{Type: t, Key: binary.LittleEndian.Uint64(op.Key), Value: op.Value})
			}
			if len(batch) == 0 {
				return nil
			}
			return dst.BatchWrite(batch)
		}
		return nil
	})
	return n, err
}

// highestOldLSN returns the highest LSN the old files used.
func highestOldLSN(walPath string, ckptLSN uint64) uint64 {
	highest := ckptLSN
	for _, p := range []string{walPath + ".bak", walPath} {
		if rep, err := wal.Replay(p, func(*wal.Record) error { return nil }); err == nil && rep.LastLSN > highest {
			highest = rep.LastLSN
		}
	}
	return highest
}

func removeMigrationTemps(tmpDB, tmpWAL string) {
	for _, p := range []string{tmpDB, storage.JournalPath(tmpDB), storage.SpillPath(tmpDB), tmpWAL, tmpWAL + ".bak"} {
		_ = os.Remove(p)
	}
}
