package wal

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
)

func frameSize(rec *Record) int64 {
	return int64(frameOverhead + plainHdr + len(rec.Key) + len(rec.Value))
}

// scanLogEncrypted is scanLog for an encrypted log: the highest LSN and the
// offset just past the last frame that authenticates. It stops, without
// error, at a clean end or at the first frame that does not decode or open.
func scanLogEncrypted(file *os.File, c *Cipher) (highest uint64, validEnd int64, err error) {
	off, empty, err := checkHeader(file, c)
	if err != nil || empty {
		return 0, 0, err
	}
	validEnd = off
	r := bufio.NewReaderSize(io.NewSectionReader(file, off, 1<<62), 1<<20)
	for {
		rec, derr := c.decode(r)
		if derr != nil {
			if derr == io.EOF || derr == ErrTornWrite || derr == errFrameAuth {
				return highest, validEnd, nil
			}
			return 0, 0, derr
		}
		validEnd += frameSize(rec)
		if rec.LSN > highest {
			highest = rec.LSN
		}
	}
}

// scanAndTrimEncrypted is scanAndTrim for an encrypted log. A frame that fails
// authentication with intact frames (higher LSN, authentic) after it is damage
// in the middle: ErrCorruptLog unless truncateCorrupt is set. Anything else
// after the last good frame is a torn tail and is cut. Whatever is cut that is
// not a short frame or zero fill is saved to <path>.discarded first.
func scanAndTrimEncrypted(file *os.File, path string, c *Cipher, truncateCorrupt bool) (uint64, error) {
	highest, validEnd, err := scanLogEncrypted(file, c)
	if err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size < encHeaderSize { // torn header: nothing to keep
		return 0, nil
	}
	if validEnd == size {
		return highest, nil
	}
	if !isShortFrameOrZeros(file, validEnd, size) {
		at, lsn, found, err := findLaterFrame(file, validEnd, size, highest, c)
		if err != nil {
			return 0, err
		}
		if found && !truncateCorrupt {
			return 0, fmt.Errorf("%w: %s: the record at offset %d fails authentication but an authentic record (LSN %d) follows at offset %d (tampering or damage); restore from a backup, or open with the WAL truncation option to cut the log at the damage and lose what follows", ErrCorruptLog, path, validEnd, lsn, at)
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

// isShortFrameOrZeros reports whether [from, size) is one interrupted append (a
// frame that runs to or past the end of the file) or only zeros.
func isShortFrameOrZeros(file *os.File, from, size int64) bool {
	if size-from < frameClear {
		return true
	}
	var h [4]byte
	if _, err := file.ReadAt(h[:], from); err != nil {
		return false
	}
	if total := int64(binary.LittleEndian.Uint32(h[:])); total >= frameOverhead+plainHdr && total <= MaxFrameSize+frameOverhead && from+total >= size {
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

// findLaterFrame searches the bytes after a bad frame for a frame that
// authenticates and has an LSN above highest. The tag is the proof: random
// bytes do not pass it.
func findLaterFrame(file *os.File, from, size int64, highest uint64, c *Cipher) (at int64, lsn uint64, found bool, err error) {
	n := min(size-from, maxResyncScan)
	buf := make([]byte, n)
	if _, err := file.ReadAt(buf, from); err != nil && err != io.EOF {
		return 0, 0, false, err
	}
	for off := 1; off+frameClear <= len(buf); off++ {
		total := int(binary.LittleEndian.Uint32(buf[off:]))
		if total < frameOverhead+plainHdr || total > MaxFrameSize+frameOverhead || off+total > len(buf) {
			continue
		}
		if binary.LittleEndian.Uint64(buf[off+4:]) <= highest {
			continue
		}
		if rec, derr := c.decode(bytes.NewReader(buf[off : off+total])); derr == nil {
			return from + int64(off), rec.LSN, true, nil
		}
	}
	return 0, 0, false, nil
}
