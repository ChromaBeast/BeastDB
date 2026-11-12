package wal

import (
	"os"
	"path/filepath"
)

// Close flushes buffered writes and closes the log file.
func (w *WAL) Close() error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.file.Sync()
	return w.file.Close()
}

// Checkpoint rotates the WAL: syncs the current file, archives it as <path>.bak,
// then opens a fresh append-only log starting from the next LSN.
// The caller MUST have flushed all dirty buffer pool pages to disk first so the
// archived WAL records are no longer needed for recovery.
func (w *WAL) Checkpoint() error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Sync(); err != nil {
		return err
	}
	// Everything appended so far is now durable (and about to be archived).
	w.syncedLSN = w.currentLSN
	if err := w.file.Close(); err != nil {
		return err
	}

	// Archive the old WAL; keep one generation for safety.
	archivePath := w.path + ".bak"
	_ = os.Remove(archivePath)
	if err := os.Rename(w.path, archivePath); err != nil {
		return err
	}

	// Open a fresh log file for subsequent writes.
	newFile, err := os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	w.file = newFile

	// Persist the rename and the new file's directory entry.
	if dir, err := os.Open(filepath.Dir(w.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
