package wal

import (
	"io"
	"os"
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
