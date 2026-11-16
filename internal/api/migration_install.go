package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// installMigratedFiles replaces the database at dbPath with the closed,
// fully checkpointed database at tmpDB. Its WAL is not needed (everything is
// in the checkpoint) and is removed. The order keeps every crash point safe:
//
//  1. The old data file is preserved as dbPath+suffix (a hard link, or a copy
//     where links are unsupported).
//  2. tmpDB is renamed over dbPath in one atomic step. Before this the old
//     database opens unchanged and the migration simply runs again.
//  3. The old WAL files are moved aside. A crash before this is harmless: the
//     new meta page's checkpoint LSN is above every old record, so recovery
//     skips them.
func installMigratedFiles(dbPath, walPath, tmpDB, tmpWAL, suffix string) error {
	for _, p := range []string{tmpWAL, tmpWAL + ".bak", storage.JournalPath(tmpDB), storage.SpillPath(tmpDB)} {
		_ = os.Remove(p)
	}

	bak := dbPath + suffix
	_ = os.Remove(bak)
	if err := os.Link(dbPath, bak); err != nil {
		if err := copyFile(dbPath, bak); err != nil {
			return fmt.Errorf("back up old database: %w", err)
		}
	}
	syncDirOf(dbPath)
	if err := os.Rename(tmpDB, dbPath); err != nil {
		return fmt.Errorf("install migrated database: %w", err)
	}
	syncDirOf(dbPath)

	for _, p := range []string{walPath, walPath + ".bak"} {
		if _, err := os.Stat(p); err == nil {
			_ = os.Remove(p + suffix)
			if err := os.Rename(p, p+suffix); err != nil {
				return fmt.Errorf("move old WAL aside: %w", err)
			}
		}
	}
	syncDirOf(walPath)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func syncDirOf(path string) {
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
