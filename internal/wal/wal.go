package wal

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// WAL manages the sequential append-only write-ahead log file.
//
// Durability is group-committed: Append assigns an LSN and writes the frame
// under mu (cheap), while the fsync is performed by whichever caller reaches
// syncMu first and covers every record appended up to that moment, so
// concurrent writers share one fsync. Lock order is always syncMu, then mu.
type WAL struct {
	mu          sync.Mutex
	file        *os.File
	path        string
	currentLSN  uint64
	syncOnWrite bool

	syncMu    sync.Mutex // serializes fsyncs; guards syncedLSN
	syncedLSN uint64     // highest LSN known to be fsynced
	errMu     sync.Mutex // guards syncErr (read by Append without syncMu)
	syncErr   error      // sticky: after a failed fsync the log is untrustworthy
}

// OpenWAL opens an existing WAL file or creates a new one at the specified path.
func OpenWAL(path string, syncOnWrite bool) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	w := &WAL{
		file:        file,
		path:        path,
		syncOnWrite: syncOnWrite,
	}

	// Scan existing file to determine the highest existing LSN
	lastLSN, err := findHighestLSN(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	// If active WAL is empty (e.g. immediately following a checkpoint rotation),
	// check the archived WAL generation to prevent LSN regression across restarts.
	if lastLSN == 0 {
		archivePath := path + ".bak"
		if archFile, err := os.Open(archivePath); err == nil {
			if archLSN, err := findHighestLSN(archFile); err == nil && archLSN > lastLSN {
				lastLSN = archLSN
			}
			_ = archFile.Close()
		}
	}

	w.currentLSN = lastLSN
	w.syncedLSN = lastLSN // everything already on disk at open is treated as durable
	return w, nil
}

// Append writes a record to the log file and assigns its LSN but does NOT
// fsync. The record is not durable until WaitDurable(lsn) or Sync returns.
func (w *WAL) Append(opType byte, key, value []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.loadSyncErr(); err != nil {
		return 0, err
	}

	rec := &Record{
		LSN:   w.currentLSN + 1,
		Type:  opType,
		Key:   key,
		Value: value,
	}

	encoded := EncodeRecord(rec)
	if len(encoded) > MaxFrameSize {
		return 0, ErrFrameTooLarge
	}
	if _, err := w.file.Write(encoded); err != nil {
		return 0, err
	}
	w.currentLSN = rec.LSN
	return rec.LSN, nil
}

// Write appends a state mutation record to the WAL, returning the assigned LSN.
// When syncOnWrite is set it returns only after the record is fsynced; concurrent
// callers share fsyncs (group commit).
func (w *WAL) Write(opType byte, key, value []byte) (uint64, error) {
	lsn, err := w.Append(opType, key, value)
	if err != nil {
		return 0, err
	}
	if w.syncOnWrite {
		if err := w.WaitDurable(lsn); err != nil {
			return 0, err
		}
	}
	return lsn, nil
}

// SyncOnWrite reports whether this log was opened with fsync-per-commit semantics.
func (w *WAL) SyncOnWrite() bool { return w.syncOnWrite }

func (w *WAL) loadSyncErr() error {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.syncErr
}

// WaitDurable blocks until the record with the given LSN has been fsynced.
// If another caller's fsync already covered it, no further fsync is issued.
func (w *WAL) WaitDurable(lsn uint64) error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	if err := w.loadSyncErr(); err != nil {
		return err
	}
	if w.syncedLSN >= lsn {
		return nil
	}
	return w.syncLocked()
}

// syncLocked fsyncs everything appended so far. Caller holds syncMu.
func (w *WAL) syncLocked() error {
	w.mu.Lock()
	target := w.currentLSN
	f := w.file
	w.mu.Unlock()

	// Appends may proceed while fsync runs; they simply miss this sync and are
	// covered by the next one. target only claims what was written before it.
	if err := f.Sync(); err != nil {
		w.errMu.Lock()
		w.syncErr = err
		w.errMu.Unlock()
		return err
	}
	if target > w.syncedLSN {
		w.syncedLSN = target
	}
	return nil
}

// Sync forces all buffered kernel writes to physical non-volatile storage.
func (w *WAL) Sync() error {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	if err := w.loadSyncErr(); err != nil {
		return err
	}
	return w.syncLocked()
}

// SyncIfEnabled fsyncs only when the log was opened with syncOnWrite.
func (w *WAL) SyncIfEnabled() error {
	if !w.syncOnWrite {
		return nil
	}
	return w.Sync()
}

// DurableLSN returns the highest LSN known to be fsynced.
func (w *WAL) DurableLSN() uint64 {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	return w.syncedLSN
}

// CurrentLSN returns the latest assigned Log Sequence Number.
func (w *WAL) CurrentLSN() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentLSN
}

// SetCurrentLSN advances currentLSN to at least targetLSN to preserve monotonicity.
func (w *WAL) SetCurrentLSN(targetLSN uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if targetLSN > w.currentLSN {
		w.currentLSN = targetLSN
	}
}

// Path returns the physical path of the WAL file.
func (w *WAL) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

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

func findHighestLSN(file *os.File) (uint64, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	var highest uint64
	for {
		rec, err := DecodeRecord(file)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF || err == ErrTornWrite {
				break
			}
			return 0, err
		}
		if rec.LSN > highest {
			highest = rec.LSN
		}
	}

	// Reset write offset back to end of file
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return 0, err
	}
	return highest, nil
}

// FileSize returns the current byte size of the WAL file on disk.
func (w *WAL) FileSize() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if info, err := w.file.Stat(); err == nil {
		return info.Size()
	}
	return 0
}
