package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
)

// RestoreResult captures recovery timing and metadata from a restore drill.
type RestoreResult struct {
	Duration    time.Duration
	DestDB      string
	DestWAL     string
	RecordCount int
}

func runRestore(backupDir, destDir string, force bool) (*RestoreResult, error) {
	if backupDir == "" {
		return nil, errors.New("backup directory is required (--backup)")
	}
	if destDir == "" {
		return nil, errors.New("destination directory is required (--dest)")
	}

	startTime := time.Now()

	// 1. Verify backup integrity before touching destination
	if err := runVerify(backupDir); err != nil {
		return nil, fmt.Errorf("backup verification failed before restore: %w", err)
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

	// 4. Verify that restored database can be opened cleanly by engine
	engine, err := api.NewEngine(destDB, destWAL, 20)
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
	fmt.Printf("Restore completed successfully in %v:\n  Destination DB: %s\n  Records loaded: %d\n", elapsed, destDB, recordCount)

	return &RestoreResult{
		Duration:    elapsed,
		DestDB:      destDB,
		DestWAL:     destWAL,
		RecordCount: recordCount,
	}, nil
}
