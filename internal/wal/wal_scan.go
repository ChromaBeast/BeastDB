package wal

import (
	"bufio"
	"io"
	"log"
	"os"
)

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
// tail (a partial frame or zero fill). If they are neither, the log may be
// damaged in the middle, so they are saved next to the WAL before the cut
// rather than destroyed.
func scanAndTrim(file *os.File, path string) (uint64, error) {
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
		saved := path + ".discarded"
		if err := copyRange(file, validEnd, size, saved); err != nil {
			return 0, err
		}
		log.Printf("wal: %d bytes after offset %d of %s are not a torn tail; saved to %s before truncating", size-validEnd, validEnd, path, saved)
	}
	if err := file.Truncate(validEnd); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
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
