package api

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	// ErrInvalidSnapshotDir indicates an empty snapshot directory path.
	ErrInvalidSnapshotDir = errors.New("snapshot: destination directory cannot be empty")
)

// CreateSnapshot takes an online, consistent physical snapshot of the database file.
// It checkpoints the buffer pool, persists metadata to Page 0, and safely clones
// the physical database file to destDir.
func (e *Engine) CreateSnapshot(destDir string) (string, error) {
	if destDir == "" {
		return "", ErrInvalidSnapshotDir
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("20060102-150405.000")
	snapshotPath := filepath.Join(destDir, fmt.Sprintf("beast-snapshot-%s.bin", timestamp))

	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Force all dirty buffer pool frames to disk, then the Meta Page
	if err := e.persistCheckpointLocked(e.wal.CurrentLSN()); err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}

	// 2. Perform physical file clone of the flushed .bin file
	src, err := os.Open(e.disk.Path())
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(snapshotPath)
		return "", err
	}

	if err := dst.Sync(); err != nil {
		_ = os.Remove(snapshotPath)
		return "", err
	}

	return snapshotPath, nil
}
