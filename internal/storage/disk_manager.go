package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrDiskInvalidPageID  = errors.New("disk: page id exceeds file bounds")
	ErrDiskBufferTooSmall = errors.New("disk: buffer smaller than 4096 bytes")
)

// File is the subset of *os.File the disk manager uses. Crash tests substitute
// an in-memory file that can drop writes that were never synced.
type File interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Sync() error
	Close() error
	Stat() (fs.FileInfo, error)
	Name() string
}

// DiskManager owns the data file and its two companions.
//
// Durability model: the data file only ever holds a complete checkpoint image.
// Pages evicted between checkpoints go to the spill file, which is scratch
// space (never fsynced, discarded on restart). WriteCheckpoint replaces the
// image atomically: every page is first written and fsynced to the journal,
// then copied into the data file, and only then is the journal emptied. On
// open, a complete journal is re-applied and an incomplete one discarded.
//
// Recovery therefore always starts from exactly the state the last checkpoint
// captured and redoes the WAL from there, which logical replay requires: it
// is not idempotent against pages that already contain later changes.
type DiskManager struct {
	mu       sync.Mutex
	file     File // data file: a checkpoint image
	journal  File // empty except while a checkpoint is being installed
	spill    File // evicted pages since the last checkpoint; not durable
	numPages uint64

	spillSlots map[uint64]int64 // pageID -> slot in the spill file
	spillNext  int64            // next free spill slot

	removeOnClose string // spill file path, removed on Close ("" for injected files)

	pc      *PageCipher // nil for a plaintext database
	scratch []byte      // PageSize sealing buffer; used under mu (encrypted only)
}

// JournalPath and SpillPath name the companion files of a data file.
func JournalPath(dbPath string) string { return dbPath + "-journal" }
func SpillPath(dbPath string) string   { return dbPath + "-spill" }

// OpenDiskManager opens or creates a database file and its journal and spill files.
func OpenDiskManager(path string) (*DiskManager, error) {
	return OpenDiskManagerEncrypted(path, nil)
}

// OpenDiskManagerEncrypted is OpenDiskManager with page encryption (ADR 0002).
// A nil cipher means plaintext. The file's format must match: an encrypted
// file without a cipher returns ErrEncryptedNoKey, a plaintext one with a
// cipher returns ErrNotEncrypted.
func OpenDiskManagerEncrypted(path string, pc *PageCipher) (*DiskManager, error) {
	data, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	journal, err := os.OpenFile(JournalPath(path), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		data.Close()
		return nil, err
	}
	spill, err := os.OpenFile(SpillPath(path), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		data.Close()
		journal.Close()
		return nil, err
	}
	syncDir(path) // make the new files' directory entries durable

	d, err := OpenDiskManagerFilesEncrypted(data, journal, spill, pc)
	if err != nil {
		data.Close()
		journal.Close()
		spill.Close()
		return nil, err
	}
	d.removeOnClose = SpillPath(path)
	return d, nil
}

// OpenDiskManagerFiles builds a disk manager on already-open files, first
// finishing or discarding a checkpoint that a crash interrupted. The spill
// file is expected to be empty.
func OpenDiskManagerFiles(data, journal, spill File) (*DiskManager, error) {
	return OpenDiskManagerFilesEncrypted(data, journal, spill, nil)
}

// OpenDiskManagerFilesEncrypted is OpenDiskManagerFiles with an optional page cipher.
func OpenDiskManagerFilesEncrypted(data, journal, spill File, pc *PageCipher) (*DiskManager, error) {
	d := &DiskManager{
		file:       data,
		journal:    journal,
		spill:      spill,
		spillSlots: make(map[uint64]int64),
		pc:         pc,
	}
	if pc != nil {
		d.scratch = make([]byte, PageSize)
	}
	if _, err := d.installJournalLocked(); err != nil {
		return nil, err
	}
	info, err := data.Stat()
	if err != nil {
		return nil, err
	}
	d.numPages = uint64(info.Size() / PageSize)
	if err := d.checkFormat(); err != nil {
		return nil, err
	}
	return d, nil
}

// checkFormat compares the file's format (read from page 0's clear tail) with
// the configured cipher. An empty file matches either.
func (d *DiskManager) checkFormat() error {
	if d.numPages == 0 {
		return nil
	}
	var p [PageSize]byte
	if _, err := d.file.ReadAt(p[:], 0); err != nil {
		return err
	}
	enc := IsEncryptedPage(p[:])
	switch {
	case enc && d.pc == nil:
		return ErrEncryptedNoKey
	case !enc && d.pc != nil:
		return ErrNotEncrypted
	}
	return nil
}

// Encrypted reports whether this disk manager seals pages.
func (d *DiskManager) Encrypted() bool { return d.pc != nil }

// KeyID returns the key generation of an encrypted database, or 0.
func (d *DiskManager) KeyID() uint16 {
	if d.pc == nil {
		return 0
	}
	return d.pc.KeyID()
}

// PageEnd is where tuple data may end on a page: PageSize, or PageDataEnd
// when the tail is reserved for the encryption tag.
func (d *DiskManager) PageEnd() int {
	if d.pc != nil {
		return PageDataEnd
	}
	return PageSize
}

// readPhysical reads one physical page into buf (PageSize bytes). unwritten is
// true when the page lies wholly past the end of src.
func readPhysical(src File, buf []byte, offset int64) (unwritten bool, err error) {
	n, err := src.ReadAt(buf[:PageSize], offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if n == 0 {
		clear(buf[:PageSize])
		return true, nil
	}
	if n < PageSize {
		return false, io.ErrUnexpectedEOF
	}
	return false, nil
}

// ReadPage reads the current version of a page: from the spill file if it was
// evicted since the last checkpoint, otherwise from the data file. A page past
// the end of the data file reads as zeros.
func (d *DiskManager) ReadPage(pageID uint64, pageData []byte) error {
	if len(pageData) < PageSize {
		return ErrDiskBufferTooSmall
	}

	d.mu.Lock()
	src, offset := d.file, int64(pageID*PageSize)
	if slot, ok := d.spillSlots[pageID]; ok {
		src, offset = d.spill, slot*PageSize
	}
	if d.pc == nil {
		defer d.mu.Unlock()
		_, err := src.ReadAt(pageData[:PageSize], offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	}
	unwritten, err := readPhysical(src, pageData, offset)
	d.mu.Unlock() // decrypt outside the lock: concurrent misses do not serialize on AES-GCM
	if err != nil {
		return fmt.Errorf("%w: page %d: %v", ErrPageAuth, pageID, err)
	}
	if unwritten {
		return nil // never written: reads as zeros, as in a plaintext database
	}
	if err := d.pc.Open(pageData[:PageSize], pageID); err != nil {
		return fmt.Errorf("%w: page %d", err, pageID)
	}
	return nil
}

// AllocatePage hands out the next page ID. The page reaches the data file at
// the next checkpoint (every allocated page is dirty until then).
func (d *DiskManager) AllocatePage() (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	pageID := d.numPages
	d.numPages++
	return pageID, nil
}

// NumPages returns the number of allocated pages.
func (d *DiskManager) NumPages() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.numPages
}

// Sync flushes the data file. Checkpoints already sync; this is for callers
// that copy the file.
func (d *DiskManager) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file.Sync()
}

// Close releases the files and removes the spill file. Pages not written by
// a checkpoint are dropped, as after a crash; callers checkpoint first.
func (d *DiskManager) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	err := d.file.Sync()
	for _, f := range []File{d.file, d.journal, d.spill} {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if d.removeOnClose != "" {
		_ = os.Remove(d.removeOnClose)
	}
	return err
}

// Path returns the physical filesystem path of the database file.
func (d *DiskManager) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file.Name()
}

// syncDir fsyncs the directory holding path so newly created files survive a
// crash. Best effort: not every platform can fsync a directory.
func syncDir(path string) {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return
	}
	_ = dir.Sync()
	_ = dir.Close()
}

// FileEncrypted inspects a data file's clear page tail. exists is false for a
// missing or empty file, which matches either format.
func FileEncrypted(path string) (exists, encrypted bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	defer f.Close()
	var p [PageSize]byte
	n, err := f.ReadAt(p[:], 0)
	if n == 0 && (err == nil || errors.Is(err, io.EOF)) {
		return false, false, nil
	}
	if n < PageSize {
		return true, false, nil
	}
	return true, IsEncryptedPage(p[:]), nil
}

// VerifyCipher returns an independent cipher over the same key for checking
// files (backup verification) concurrently with normal operation, or nil for
// a plaintext database. A PageCipher itself is not safe for concurrent use.
func (d *DiskManager) VerifyCipher() *PageCipher {
	if d.pc == nil {
		return nil
	}
	return NewPageCipher(d.pc.aead, d.pc.keyID)
}
