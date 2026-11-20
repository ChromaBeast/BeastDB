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

// CreateSnapshot takes an online, consistent physical snapshot of the database
// file without holding the engine lock for the copy (ADR 0005).
//
// Under the lock it makes a checkpoint, which leaves the data file holding a
// complete image of the database as of that moment, then releases the lock and
// copies the file. Reads and writes carry on during the copy: writes go to the
// WAL and the buffer pool, and pages evicted from the pool go to the spill
// file, so nothing touches the data file. The only writers of the data file
// are checkpoints and Close; imageMu keeps them out until the copy is done, so
// the file is stable while it is read. The snapshot holds exactly the writes
// committed before the checkpoint; later ones are not in it.
func (e *Engine) CreateSnapshot(destDir string) (string, error) {
	if destDir == "" {
		return "", ErrInvalidSnapshotDir
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	timestamp := time.Now().Format("20060102-150405.000")
	snapshotPath := filepath.Join(destDir, fmt.Sprintf("beast-snapshot-%s.bin", timestamp))

	e.imageMu.Lock() // no other checkpoint, and no Close, until the copy ends
	defer e.imageMu.Unlock()

	srcPath, err := e.checkpointForSnapshot()
	if err != nil {
		return "", err
	}
	if e.snapshotHook != nil {
		e.snapshotHook() // tests: hold the engine between checkpoint and copy
	}
	if err := copyFileSynced(srcPath, snapshotPath); err != nil {
		_ = os.Remove(snapshotPath)
		return "", err
	}
	return snapshotPath, nil
}

// checkpointForSnapshot checkpoints under the engine lock and returns the data
// file's path. This is the only part of a snapshot that pauses the engine; its
// length depends on the dirty pages to flush, not on the size of the file.
func (e *Engine) checkpointForSnapshot() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return "", e.failed
	}
	// Data pages must not reach the data file ahead of their WAL records.
	if err := e.wal.SyncIfEnabled(); err != nil {
		return "", fmt.Errorf("snapshot: sync wal: %w", err)
	}
	if err := e.persistCheckpointLocked(e.wal.CurrentLSN()); err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	return e.disk.Path(), nil
}

func copyFileSynced(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}
