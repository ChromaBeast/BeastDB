package storage

import (
	"errors"
	"io"
	"os"
	"sync"
)

var (
	ErrDiskInvalidPageID  = errors.New("disk: page id exceeds file bounds")
	ErrDiskBufferTooSmall = errors.New("disk: buffer smaller than 4096 bytes")
)

// DiskManager manages physical read/write operations against the database file.
type DiskManager struct {
	file     *os.File
	numPages uint64
	mu       sync.Mutex
}

// OpenDiskManager opens or creates a database file for 4KB aligned page I/O.
func OpenDiskManager(path string) (*DiskManager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	numPages := uint64(info.Size() / PageSize)

	return &DiskManager{
		file:     file,
		numPages: numPages,
	}, nil
}

// ReadPage reads a 4KB block from disk at offset pageID * PageSize.
func (d *DiskManager) ReadPage(pageID uint64, pageData []byte) error {
	if len(pageData) < PageSize {
		return ErrDiskBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	offset := int64(pageID * PageSize)
	_, err := d.file.ReadAt(pageData[:PageSize], offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// WritePage writes a 4KB block to disk at offset pageID * PageSize and flushes.
func (d *DiskManager) WritePage(pageID uint64, pageData []byte) error {
	if len(pageData) < PageSize {
		return ErrDiskBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	offset := int64(pageID * PageSize)
	if _, err := d.file.WriteAt(pageData[:PageSize], offset); err != nil {
		return err
	}

	return d.file.Sync()
}

// AllocatePage atomically reserves the next 4KB slot in the database file.
func (d *DiskManager) AllocatePage() (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pageID := d.numPages
	newSize := int64((pageID + 1) * PageSize)

	if err := d.file.Truncate(newSize); err != nil {
		return 0, err
	}

	d.numPages++
	return pageID, nil
}

// NumPages returns the total page count managed on disk.
func (d *DiskManager) NumPages() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.numPages
}

// Sync flushes all pending file writes to durable physical storage.
func (d *DiskManager) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file.Sync()
}

// Close flushes and releases the database file handle.
func (d *DiskManager) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.file.Sync(); err != nil {
		d.file.Close()
		return err
	}
	return d.file.Close()
}
