package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// flagSet reports whether the named flag was given on the command line.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// runRestoreNew restores a bundle into a data directory that does not exist
// yet. It cannot damage a database: it refuses any directory that already
// holds something, builds and checks the restored database in a hidden staging
// directory beside the target, and renames it into place only after the
// database has been verified, opened by the engine and scanned. A failure at
// any step leaves the target as it was.
func runRestoreNew(backupDir, newDir, keyFile string) error {
	if backupDir == "" || newDir == "" {
		return errors.New("--backup and --new-dir are required")
	}
	target, err := filepath.Abs(newDir)
	if err != nil {
		return err
	}
	bundle, err := filepath.Abs(backupDir)
	if err != nil {
		return err
	}
	if target == bundle || isWithin(target, bundle) {
		return errors.New("--new-dir must not be the backup bundle or inside it")
	}
	if err := requireEmptyOrMissing(target); err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".beastdb-restore-")
	if err != nil {
		return fmt.Errorf("cannot stage next to %s: %w", target, err)
	}
	defer os.RemoveAll(stage) // gone after the rename; cleans up after any failure

	res, err := restoreInto(bundle, stage, false, keyFile)
	if err != nil {
		return err
	}
	// Re-check at the last moment: another process may have used the name.
	if err := requireEmptyOrMissing(target); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err // Remove only deletes an empty directory
	}
	if err := os.Rename(stage, target); err != nil {
		return fmt.Errorf("install restored directory: %w", err)
	}
	fmt.Printf("Restored into new data directory %s in %v (%d records, verified and opened cleanly).\n", target, res.Duration, res.RecordCount)
	hint := "<server binary> -data-dir " + target
	if keyFile != "" {
		hint += " -encryption-key-file " + keyFile
	}
	fmt.Println("Start a server on it with:\n  " + hint)
	return nil
}

func requireEmptyOrMissing(dir string) error {
	// Stat first: on Windows, reading a regular file as a directory reports
	// "not exist", which would let a file at the target be treated as missing.
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return fmt.Errorf("%s exists and is not a directory (it must be a directory that does not exist yet)", dir)
	}
	ents, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("%s: %w (it must be a directory that does not exist yet)", dir, err)
	case len(ents) > 0:
		return fmt.Errorf("%s already exists and is not empty; --new-dir never overwrites. Choose a new path, or use --dest with --force on a stopped server", dir)
	}
	return nil
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
