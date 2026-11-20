package wal

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

// ErrCorruptLog reports a WAL that is damaged in the middle: a record fails
// its checks but intact records follow it. Those later records were written,
// and possibly acknowledged, so the log is not cut silently.
var ErrCorruptLog = errors.New("wal: log is damaged in the middle")

// maxResyncScan bounds how many bytes after a bad record are searched for a
// later intact record.
const maxResyncScan = 256 << 20

// scanLog reads records from the start of file and returns the highest LSN
// and the offset just past the last valid record. It stops, without error, at
// a clean end of file or at the first record that fails to decode.
func scanLog(file *os.File) (highest uint64, validEnd int64, err error) {
	r := bufio.NewReaderSize(io.NewSectionReader(file, 0, 1<<62), 1<<20)
	for {
		rec, derr := DecodeRecord(r)
		if derr != nil {
			if isLogEnd(derr) {
				return highest, validEnd, nil
			}
			return 0, 0, derr
		}
		validEnd += int64(HeaderSize + len(rec.Key) + len(rec.Value))
		if rec.LSN > highest {
			highest = rec.LSN
		}
	}
}

// isLogEnd reports whether a decode error marks the end of the usable log:
// a clean EOF or a record that was never completely written.
func isLogEnd(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF || err == ErrTornWrite || err == ErrRecordTooSmall
}

// scanAndTrim returns the highest LSN in file and truncates it to the last
// valid record. Bytes that follow an undecodable record are normally a torn
// tail (a partial frame or zero fill). If an intact record with a higher LSN
// follows the damage, the log is damaged in the middle: that fails with
// ErrCorruptLog unless truncateCorrupt is set. Anything cut that is not a
// plain torn tail is saved next to the WAL first.
func scanAndTrim(file *os.File, path string, truncateCorrupt bool) (uint64, error) {
	highest, validEnd, err := scanLog(file)
	if err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if validEnd == size {
		return highest, nil
	}

	if !isPlainTornTail(file, validEnd, size) {
		at, lsn, found, err := findLaterRecord(file, validEnd, size, highest)
		if err != nil {
			return 0, err
		}
		if found && !truncateCorrupt {
			return 0, fmt.Errorf("%w: %s: the record at offset %d is damaged but an intact record (LSN %d) follows at offset %d; restore from a backup, or open with the WAL truncation option to cut the log at the damage and lose what follows", ErrCorruptLog, path, validEnd, lsn, at)
		}
		saved := path + ".discarded"
		if err := copyRange(file, validEnd, size, saved); err != nil {
			return 0, err
		}
		log.Printf("wal: %d bytes after offset %d of %s are not a torn tail; saved to %s before truncating", size-validEnd, validEnd, path, saved)
	}
	if err := truncateFile(path, validEnd); err != nil {
		return 0, err
	}
	return highest, nil
}

// isPlainTornTail reports whether the bytes in [from, size) look like one
// interrupted append: a frame that runs to or past the end of the file, or
// nothing but zeros.
func isPlainTornTail(file *os.File, from, size int64) bool {
	if size-from < HeaderSize {
		return true
	}
	var header [4]byte
	if _, err := file.ReadAt(header[:], from); err != nil {
		return false
	}
	recLen := int64(header[0]) | int64(header[1])<<8 | int64(header[2])<<16 | int64(header[3])<<24
	if recLen >= HeaderSize && recLen <= MaxFrameSize && from+recLen >= size {
		return true
	}
	buf := make([]byte, 64<<10)
	for off := from; off < size; {
		n, err := file.ReadAt(buf[:min(int64(len(buf)), size-off)], off)
		for _, b := range buf[:n] {
			if b != 0 {
				return false
			}
		}
		off += int64(n)
		if err != nil {
			return off >= size
		}
	}
	return true
}

func copyRange(file *os.File, from, to int64, dstPath string) error {
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, io.NewSectionReader(file, from, to-from)); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

// findLaterRecord searches the bytes after a bad record for an intact record
// with an LSN above highest, which proves the damage is not a torn tail.
func findLaterRecord(file *os.File, from, size int64, highest uint64) (at int64, lsn uint64, found bool, err error) {
	n := min(size-from, maxResyncScan)
	buf := make([]byte, n)
	if _, err := file.ReadAt(buf, from); err != nil && !errors.Is(err, io.EOF) {
		return 0, 0, false, err
	}
	for off := 1; off+HeaderSize <= len(buf); off++ {
		recLen := int(uint32(buf[off]) | uint32(buf[off+1])<<8 | uint32(buf[off+2])<<16 | uint32(buf[off+3])<<24)
		// Only records whose CRC covers the header count as proof: their
		// checks are strong enough that garbage will not pass by chance.
		if recLen < HeaderSize || off+recLen > len(buf) || buf[off+12]&headerCRCFlag == 0 {
			continue
		}
		rec, derr := DecodeRecord(bytes.NewReader(buf[off : off+recLen]))
		if derr == nil && rec.LSN > highest {
			return from + int64(off), rec.LSN, true, nil
		}
	}
	return 0, 0, false, nil
}

// truncateFile cuts the file at path to size and syncs it through a separate,
// non-append handle. The log itself is open with O_APPEND, and on Windows an
// append-only handle has no right to change the file size ("Access is
// denied"). The append handle keeps working afterwards: every write goes to
// the current end of the file.
func truncateFile(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
