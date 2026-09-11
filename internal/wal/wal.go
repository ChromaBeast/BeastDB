package wal

import (
	"io"
	"os"
	"sync"
)

// WAL manages the sequential append-only write-ahead log file.
type WAL struct {
	mu          sync.Mutex
	file        *os.File
	path        string
	currentLSN  uint64
	syncOnWrite bool
}

// OpenWAL opens an existing WAL file or creates a new one at the specified path.
func OpenWAL(path string, syncOnWrite bool) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
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
	w.currentLSN = lastLSN

	return w, nil
}

// Write appends a state mutation record to the WAL, returning the assigned LSN.
func (w *WAL) Write(opType byte, key, value []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.currentLSN++
	rec := &Record{
		LSN:   w.currentLSN,
		Type:  opType,
		Key:   key,
		Value: value,
	}

	encoded := EncodeRecord(rec)
	if _, err := w.file.Write(encoded); err != nil {
		return 0, err
	}

	if w.syncOnWrite {
		if err := w.file.Sync(); err != nil {
			return 0, err
		}
	}

	return w.currentLSN, nil
}

// Sync forces all buffered kernel writes to physical non-volatile storage.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

// CurrentLSN returns the latest assigned Log Sequence Number.
func (w *WAL) CurrentLSN() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentLSN
}

// Close flushes buffered writes and closes the log file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.file.Sync()
	return w.file.Close()
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
