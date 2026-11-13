package storage

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

// Journal layout:
//
//	header  : magic "BDBJRNL1" | page count (uint64 LE)
//	entries : page ID (uint64 LE) | 4096-byte page image   (count times)
//	trailer : CRC-32 (IEEE, uint32 LE) over header and entries | "JEND"
//
// A journal is complete only if its size, magic, trailer and CRC all agree.
// The data file is written only after a complete journal has been fsynced, so
// an incomplete journal means the data file was not touched and can be discarded.
const (
	journalMagic       = "BDBJRNL1"
	journalEnd         = "JEND"
	journalHeaderSize  = 16
	journalEntrySize   = 8 + PageSize
	journalTrailerSize = 8
	journalBufSize     = 1 << 20
)

var errJournalNotInstalled = errors.New("disk: checkpoint journal failed verification after write")

// PageImage is one page to install at a checkpoint.
type PageImage struct {
	ID   uint64
	Data []byte
}

// WriteCheckpoint atomically replaces the data file's image with the current
// state: pages (the buffer pool's dirty frames, which win over any spilled
// copy) plus every page in the spill file. Either all of it reaches the data
// file or, after a crash, none of it does.
func (d *DiskManager) WriteCheckpoint(pages []PageImage) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// A journal left complete by an earlier checkpoint that failed part-way must
	// be installed first; otherwise its pages and this one's would mix.
	if _, err := d.installJournalLocked(); err != nil {
		return err
	}

	inPool := make(map[uint64]struct{}, len(pages))
	for _, p := range pages {
		inPool[p.ID] = struct{}{}
	}
	var spilled []uint64
	for id := range d.spillSlots {
		if _, ok := inPool[id]; !ok {
			spilled = append(spilled, id)
		}
	}
	if len(pages)+len(spilled) == 0 {
		return nil
	}

	if err := d.writeJournalLocked(pages, spilled); err != nil {
		return err
	}
	installed, err := d.installJournalLocked()
	if err != nil {
		return err
	}
	if !installed {
		return errJournalNotInstalled
	}
	return d.resetSpillLocked()
}

// writeJournalLocked writes and fsyncs a complete journal. Caller holds d.mu.
func (d *DiskManager) writeJournalLocked(pages []PageImage, spilled []uint64) error {
	bw := bufio.NewWriterSize(io.NewOffsetWriter(d.journal, 0), journalBufSize)
	sum := crc32.NewIEEE()
	w := io.MultiWriter(bw, sum)

	var hdr [journalHeaderSize]byte
	copy(hdr[:8], journalMagic)
	binary.LittleEndian.PutUint64(hdr[8:], uint64(len(pages)+len(spilled)))
	_, _ = w.Write(hdr[:])

	var id [8]byte
	for _, p := range pages {
		if len(p.Data) < PageSize {
			return ErrDiskBufferTooSmall
		}
		binary.LittleEndian.PutUint64(id[:], p.ID)
		_, _ = w.Write(id[:])
		_, _ = w.Write(p.Data[:PageSize])
	}
	buf := make([]byte, PageSize)
	for _, pid := range spilled {
		clear(buf)
		if _, err := d.spill.ReadAt(buf, d.spillSlots[pid]*PageSize); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		binary.LittleEndian.PutUint64(id[:], pid)
		_, _ = w.Write(id[:])
		_, _ = w.Write(buf)
	}

	var trailer [journalTrailerSize]byte
	binary.LittleEndian.PutUint32(trailer[:4], sum.Sum32())
	copy(trailer[4:], journalEnd)
	_, _ = bw.Write(trailer[:])
	if err := bw.Flush(); err != nil { // reports the first write error, if any
		return err
	}
	return d.journal.Sync()
}

// installJournalLocked copies a complete journal into the data file, fsyncs
// it, then empties the journal durably. An incomplete journal is discarded.
// It reports whether a journal was installed. Safe to repeat after a crash at
// any point: re-applying full page images is idempotent. Caller holds d.mu
// (or has exclusive access during open).
func (d *DiskManager) installJournalLocked() (bool, error) {
	info, err := d.journal.Stat()
	if err != nil {
		return false, err
	}
	size := info.Size()
	if size == 0 {
		return false, nil
	}

	count, complete, err := d.verifyJournalLocked(size)
	if err != nil {
		return false, err
	}
	if complete {
		r := bufio.NewReaderSize(io.NewSectionReader(d.journal, journalHeaderSize, int64(count)*journalEntrySize), journalBufSize)
		var id [8]byte
		page := make([]byte, PageSize)
		for i := uint64(0); i < count; i++ {
			if _, err := io.ReadFull(r, id[:]); err != nil {
				return false, err
			}
			if _, err := io.ReadFull(r, page); err != nil {
				return false, err
			}
			if _, err := d.file.WriteAt(page, int64(binary.LittleEndian.Uint64(id[:])*PageSize)); err != nil {
				return false, err
			}
		}
		if err := d.file.Sync(); err != nil {
			return false, err
		}
	}
	// Empty the journal durably (truncate, not delete, so this also holds where
	// a directory cannot be fsynced): a stale journal re-applied after a later
	// checkpoint would roll pages back.
	if err := d.journal.Truncate(0); err != nil {
		return false, err
	}
	if err := d.journal.Sync(); err != nil {
		return false, err
	}
	return complete, nil
}

// verifyJournalLocked checks the journal's framing and checksum and returns
// its page count. complete is false for a journal that was never fully written.
func (d *DiskManager) verifyJournalLocked(size int64) (count uint64, complete bool, err error) {
	if size < journalHeaderSize+journalTrailerSize {
		return 0, false, nil
	}
	var hdr [journalHeaderSize]byte
	if _, err := d.journal.ReadAt(hdr[:], 0); err != nil {
		return 0, false, err
	}
	if !bytes.Equal(hdr[:8], []byte(journalMagic)) {
		return 0, false, nil
	}
	count = binary.LittleEndian.Uint64(hdr[8:])
	body := size - journalHeaderSize - journalTrailerSize
	if count > uint64(body)/journalEntrySize || int64(count)*journalEntrySize != body {
		return 0, false, nil
	}

	sum := crc32.NewIEEE()
	if _, err := io.Copy(sum, io.NewSectionReader(d.journal, 0, size-journalTrailerSize)); err != nil {
		return 0, false, err
	}
	var trailer [journalTrailerSize]byte
	if _, err := d.journal.ReadAt(trailer[:], size-journalTrailerSize); err != nil && !errors.Is(err, io.EOF) {
		return 0, false, err
	}
	if string(trailer[4:]) != journalEnd || binary.LittleEndian.Uint32(trailer[:4]) != sum.Sum32() {
		return 0, false, nil
	}
	return count, true, nil
}
