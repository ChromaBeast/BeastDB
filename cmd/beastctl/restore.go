package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// RestoreResult captures recovery timing and metadata from a restore drill.
type RestoreResult struct {
	Duration    time.Duration
	DestDB      string
	DestWAL     string
	RecordCount int
}

func runRestore(backupDir, destDir string, force bool) (*RestoreResult, error) {
	return runRestoreKey(backupDir, destDir, force, "")
}

func runRestoreKey(backupDir, destDir string, force bool, keyFile string) (*RestoreResult, error) {
	res, err := restoreInto(backupDir, destDir, force, keyFile)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Restore completed successfully in %v:\n  Destination DB: %s\n  Records loaded: %d\n", res.Duration, res.DestDB, res.RecordCount)
	return res, nil
}

// restoreInto does the work of runRestoreKey without printing a summary.
func restoreInto(backupDir, destDir string, force bool, keyFile string) (*RestoreResult, error) {
	if backupDir == "" {
		return nil, errors.New("backup directory is required (--backup)")
	}
	if destDir == "" {
		return nil, errors.New("destination directory is required (--dest)")
	}

	startTime := time.Now()

	// 1. Verify backup integrity before touching destination
	if err := runVerifyKey(backupDir, keyFile); err != nil {
		return nil, fmt.Errorf("backup verification failed before restore: %w", err)
	}

	manifest, err := readManifest(backupDir)
	if err != nil {
		return nil, err
	}
	if manifest.Encrypted && keyFile == "" {
		return nil, errors.New("backup is encrypted: --key-file is required to restore it")
	}
	if !manifest.Encrypted && keyFile != "" {
		return nil, errors.New("backup is not encrypted but --key-file was given")
	}

	destDB := filepath.Join(destDir, "beast.bin")
	destWAL := filepath.Join(destDir, "beast.wal")

	// 2. Safety check: avoid silent overwrite unless --force is specified
	if !force {
		if _, err := os.Stat(destDB); err == nil {
			return nil, fmt.Errorf("destination file %s already exists; use --force to overwrite", destDB)
		}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination dir: %w", err)
	}

	// 3. Copy data.bin to destDB
	srcPath := filepath.Join(backupDir, "data.bin")
	src, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	dst, err := os.OpenFile(destDB, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("copy to destDB failed: %w", err)
	}
	if err := dst.Sync(); err != nil {
		return nil, err
	}

	_ = os.Remove(destWAL)
	// A journal left by the previous database (e.g. a crash mid-checkpoint)
	// belongs to that database; applied to the restored file it would corrupt it.
	if err := os.Remove(storage.JournalPath(destDB)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale journal: %w", err)
	}

	// The keyring belongs to the data file: install the bundle's, or drop a
	// stale one left by the database this replaces.
	destKeyring := crypt.KeyringPath(destDB)
	if err := os.Remove(destKeyring); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if manifest.Encrypted {
		kr, err := os.ReadFile(filepath.Join(backupDir, "keyring.json"))
		if err != nil {
			return nil, fmt.Errorf("encrypted backup has no keyring.json: %w", err)
		}
		if err := os.WriteFile(destKeyring, kr, 0o600); err != nil {
			return nil, err
		}
	}

	// 4. Verify that restored database can be opened cleanly by engine
	engine, err := api.NewEngineWithOptions(destDB, destWAL, 20, api.Options{KeyFile: keyFile})
	if err != nil {
		return nil, fmt.Errorf("failed opening restored database with engine: %w", err)
	}
	defer engine.Close()

	cursor, err := engine.Scan(0, ^uint64(0))
	recordCount := 0
	if err == nil {
		for {
			_, _, ok, sErr := cursor.Next()
			if sErr != nil || !ok {
				break
			}
			recordCount++
		}
		cursor.Close()
	}

	elapsed := time.Since(startTime)

	return &RestoreResult{
		Duration:    elapsed,
		DestDB:      destDB,
		DestWAL:     destWAL,
		RecordCount: recordCount,
	}, nil
}

func readManifest(dir string) (BackupManifest, error) {
	var m BackupManifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("missing or unreadable manifest.json: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	return m, nil
}
