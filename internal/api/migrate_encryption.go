package api

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// MigrateOptions selects the direction of an offline format change (ADR 0003):
// a source key file means the database is encrypted now; a destination key file
// means it will be. Both, with different files, is a key rotation (a fresh data
// key is generated every time). Neither is rejected.
type MigrateOptions struct {
	SrcKeyFile, DstKeyFile string
	PoolSize               int
	// DeleteOriginal removes the replaced files instead of keeping them as
	// *.pre<suffix>.bak. Removal cannot guarantee the bytes are unrecoverable.
	DeleteOriginal bool
}

// MigrateResult reports what was converted and where the originals went.
type MigrateResult struct {
	Records   int
	BackupSfx string   // ".preenc.bak", ".predec.bak" or ".prekey.bak"
	Kept      []string // original files left on disk (empty with DeleteOriginal)
}

var errMigrateNoChange = errors.New("migrate: give a source and/or destination key file")

// MigrateEncryption converts the database at dbPath/walPath offline, by copying
// every record into a new database in the destination format and swapping the
// files only after the copy has been written, checkpointed and reopened. The
// server must be stopped. The copy is logical: it re-packs pages, and records
// are what moves; the WAL restarts at LSN 0, so replicas must be re-seeded.
func MigrateEncryption(dbPath, walPath string, o MigrateOptions) (MigrateResult, error) {
	var res MigrateResult
	if o.SrcKeyFile == "" && o.DstKeyFile == "" {
		return res, errMigrateNoChange
	}
	pool := o.PoolSize
	if pool <= 0 {
		pool = 256
	}
	switch {
	case o.SrcKeyFile == "":
		res.BackupSfx = ".preenc.bak"
	case o.DstKeyFile == "":
		res.BackupSfx = ".predec.bak"
	default:
		res.BackupSfx = ".prekey.bak"
	}

	var srcKeys *crypt.Keys
	if o.SrcKeyFile != "" {
		master, err := crypt.ReadMasterKey(o.SrcKeyFile)
		if err != nil {
			return res, err
		}
		if srcKeys, err = crypt.OpenKeyring(dbPath, master); err != nil {
			return res, err
		}
	} else if exists, enc, err := storage.FileEncrypted(dbPath); err != nil {
		return res, err
	} else if exists && enc {
		return res, storage.ErrEncryptedNoKey
	}

	tmpDB, tmpWAL := dbPath+".migrating", walPath+".migrating"
	cleanup := func() {
		for _, p := range []string{tmpDB, tmpWAL, storage.JournalPath(tmpDB), storage.SpillPath(tmpDB), crypt.KeyringPath(tmpDB)} {
			_ = os.Remove(p)
		}
	}
	cleanup()

	var dstKeys *crypt.Keys
	if o.DstKeyFile != "" {
		master, err := crypt.ReadMasterKey(o.DstKeyFile)
		if err != nil {
			return res, err
		}
		id := uint16(1)
		if srcKeys != nil {
			id = srcKeys.ID + 1
		}
		if dstKeys, err = crypt.CreateKeyring(tmpDB, master, id); err != nil {
			return res, err
		}
	}

	src, err := openEngineKeys(dbPath, walPath, pool, srcKeys, Options{})
	if err != nil {
		cleanup()
		return res, fmt.Errorf("open source: %w", err)
	}
	dst, err := openEngineKeys(tmpDB, tmpWAL, pool, dstKeys, Options{})
	if err != nil {
		_ = src.Close()
		cleanup()
		return res, fmt.Errorf("create destination: %w", err)
	}
	n, err := copyRecordsTo(src, dst)
	_ = src.Close()
	if err == nil {
		err = dst.Close()
	} else {
		_ = dst.Close()
	}
	if err != nil {
		cleanup()
		return res, fmt.Errorf("copy records: %w", err)
	}
	res.Records = n

	// Prove the new files open with the destination key and hold every record
	// before anything of the original is touched.
	check, err := openEngineKeys(tmpDB, tmpWAL, pool, dstKeys, Options{})
	if err != nil {
		cleanup()
		return res, fmt.Errorf("verify destination: %w", err)
	}
	got := 0
	cur, err := check.Scan(0, math.MaxUint64)
	if err == nil {
		for {
			_, _, ok, nerr := cur.Next()
			if nerr != nil || !ok {
				err = nerr
				break
			}
			got++
		}
		_ = cur.Close()
	}
	_ = check.Close()
	if err == nil && got != n {
		err = fmt.Errorf("destination holds %d records, expected %d", got, n)
	}
	if err != nil {
		cleanup()
		return res, fmt.Errorf("verify destination: %w", err)
	}

	return res, swapMigrated(dbPath, walPath, tmpDB, tmpWAL, res.BackupSfx, o.DeleteOriginal, dstKeys != nil, &res)
}

func copyRecordsTo(src, dst *Engine) (int, error) {
	cur, err := src.Scan(0, math.MaxUint64)
	if err != nil {
		return 0, err
	}
	defer cur.Close()
	var ops []BatchOperation
	size, total := 0, 0
	flush := func() error {
		if len(ops) == 0 {
			return nil
		}
		err := dst.BatchWrite(ops)
		ops, size = ops[:0], 0
		return err
	}
	for {
		key, rid, ok, err := cur.Next()
		if err != nil {
			return total, err
		}
		if !ok {
			break
		}
		v, err := src.ReadTuple(rid)
		if err != nil {
			return total, fmt.Errorf("read key %d: %w", key, err)
		}
		if size+len(v) > MaxBatchBytes/2 || len(ops) >= 1000 {
			if err := flush(); err != nil {
				return total, err
			}
		}
		ops = append(ops, BatchOperation{Type: BatchOpPut, Key: key, Value: append([]byte(nil), v...)})
		size += len(v) + 16
		total++
	}
	return total, flush()
}

// swapMigrated installs the migrated files. Each original is first preserved
// as <name><sfx> (a hard link, or a copy), then replaced by an atomic rename, so
// no step leaves a path without a file. The keyring and WAL go in before the
// data file; a crash in between fails loudly (a key or WAL format mismatch) and
// the preserved originals allow a manual restore. With deleteOrig no copy is
// kept.
func swapMigrated(dbPath, walPath, tmpDB, tmpWAL, sfx string, deleteOrig, dstEncrypted bool, res *MigrateResult) error {
	originals := []string{dbPath, walPath, walPath + ".bak", crypt.KeyringPath(dbPath)}
	if !deleteOrig {
		for _, p := range originals {
			if _, err := os.Stat(p); err != nil {
				continue
			}
			bak := p + sfx
			_ = os.Remove(bak)
			if err := os.Link(p, bak); err != nil {
				if err := copyFile(p, bak); err != nil {
					return fmt.Errorf("keep original %s: %w", p, err)
				}
			}
			res.Kept = append(res.Kept, bak)
		}
		syncDirOf(dbPath)
	}
	for _, p := range [][2]string{{crypt.KeyringPath(tmpDB), crypt.KeyringPath(dbPath)}, {tmpWAL, walPath}, {tmpDB, dbPath}} {
		if _, err := os.Stat(p[0]); err != nil {
			continue
		}
		if err := os.Rename(p[0], p[1]); err != nil {
			return fmt.Errorf("install %s: %w", p[1], err)
		}
	}
	// Originals the new database does not replace: the archived WAL generation,
	// a keyring that no longer applies (decrypt) and the journal. A decrypted
	// database must not keep the keyring of the encrypted one.
	stale := []string{walPath + ".bak", storage.JournalPath(dbPath)}
	if !dstEncrypted {
		stale = append(stale, crypt.KeyringPath(dbPath))
	}
	for _, p := range stale {
		_ = os.Remove(p)
	}
	_ = os.Remove(storage.JournalPath(tmpDB))
	_ = os.Remove(storage.SpillPath(tmpDB))
	syncDirOf(dbPath)
	return nil
}
