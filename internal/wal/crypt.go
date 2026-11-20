package wal

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// Encrypted WAL format (ADR 0003). The file starts with a 16-byte header:
//
//	magic "BDBWALE1" | key ID (uint16 LE) | 6 reserved bytes
//
// followed by one AEAD frame per record:
//
//	recLen(4) | LSN(8) | nonce(12) | ciphertext(type | keyLen | valLen | key | value) | tag(16)
//
// recLen is the whole frame and, with the LSN, stays in clear and is the AAD,
// so a frame cannot be moved to another LSN or resized. Plaintext logs have no
// header, so the two formats cannot be confused.
const (
	encMagic      = "BDBWALE1"
	encHeaderSize = 16
	frameClear    = 4 + 8
	frameOverhead = frameClear + 12 + 16
	plainHdr      = 1 + 2 + 4 // type, keyLen, valLen inside the ciphertext
)

var (
	// ErrWALTampered is ErrCorruptLog: a frame fails authentication although a
	// later frame authenticates, so it cannot be a torn tail.
	ErrWALTampered = ErrCorruptLog
	// ErrWALFormat: an encrypted engine met a plaintext log, or the reverse.
	ErrWALFormat = errors.New("wal: log encryption format does not match the database (run beastctl encrypt/decrypt)")
	errFrameAuth = errors.New("wal: frame failed authentication")
)

// Cipher seals and opens WAL frames.
type Cipher struct {
	aead  cipher.AEAD
	keyID uint16
}

// NewCipher wraps the WAL AEAD (12-byte nonce, 16-byte tag).
func NewCipher(aead cipher.AEAD, keyID uint16) *Cipher { return &Cipher{aead: aead, keyID: keyID} }

func (c *Cipher) header() []byte {
	h := make([]byte, encHeaderSize)
	copy(h, encMagic)
	binary.LittleEndian.PutUint16(h[8:], c.keyID)
	return h
}

func (c *Cipher) encode(rec *Record) ([]byte, error) {
	kl, vl := len(rec.Key), len(rec.Value)
	total := frameOverhead + plainHdr + kl + vl
	buf := make([]byte, frameClear+12, total)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(total))
	binary.LittleEndian.PutUint64(buf[4:12], rec.LSN)
	nonce := buf[frameClear : frameClear+12]
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	plain := make([]byte, plainHdr+kl+vl)
	plain[0] = rec.Type
	binary.LittleEndian.PutUint16(plain[1:3], uint16(kl))
	binary.LittleEndian.PutUint32(plain[3:7], uint32(vl))
	copy(plain[plainHdr:], rec.Key)
	copy(plain[plainHdr+kl:], rec.Value)
	return c.aead.Seal(buf, nonce, plain, buf[:frameClear]), nil
}

// decode reads one frame. io.EOF means a clean end; ErrTornWrite a short or
// absurd frame; errFrameAuth a complete frame that fails its tag.
func (c *Cipher) decode(r io.Reader) (*Record, error) {
	var clear [frameClear]byte
	if n, err := io.ReadFull(r, clear[:]); err != nil {
		if n == 0 && err == io.EOF {
			return nil, io.EOF
		}
		return nil, ErrTornWrite
	}
	total := binary.LittleEndian.Uint32(clear[0:4])
	if total < frameOverhead+plainHdr || total > MaxFrameSize+frameOverhead {
		return nil, ErrTornWrite
	}
	rest := make([]byte, total-frameClear)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, ErrTornWrite
	}
	nonce, ct := rest[:12], rest[12:]
	plain, err := c.aead.Open(nil, nonce, ct, clear[:])
	if err != nil {
		return nil, errFrameAuth
	}
	kl := int(binary.LittleEndian.Uint16(plain[1:3]))
	vl := int(binary.LittleEndian.Uint32(plain[3:7]))
	if plainHdr+kl+vl != len(plain) {
		return nil, errFrameAuth
	}
	return &Record{
		LSN:   binary.LittleEndian.Uint64(clear[4:12]),
		Type:  plain[0],
		Key:   plain[plainHdr : plainHdr+kl],
		Value: plain[plainHdr+kl:],
	}, nil
}

// scan walks the frames of r, calling fn for each authentic record. torn is
// true if it stopped at a torn tail. A frame that fails authentication is a
// torn tail only if no later frame authenticates; otherwise ErrWALTampered.
func (c *Cipher) scan(r io.Reader, fn func(*Record) error) (torn bool, err error) {
	for {
		rec, derr := c.decode(r)
		switch derr {
		case nil:
			if err := fn(rec); err != nil {
				return false, err
			}
		case io.EOF:
			return false, nil
		case errFrameAuth:
			if next, nerr := c.decode(r); nerr == nil && next != nil {
				return false, ErrWALTampered
			}
			return true, nil
		default:
			return true, nil
		}
	}
}

// checkHeader validates the file's format against c and returns the offset
// where frames start. A file shorter than the header is a torn creation and
// holds no records.
func checkHeader(f *os.File, c *Cipher) (off int64, empty bool, err error) {
	info, err := f.Stat()
	if err != nil {
		return 0, false, err
	}
	size := info.Size()
	if c == nil {
		if size >= int64(len(encMagic)) {
			var m [len(encMagic)]byte
			if _, err := f.ReadAt(m[:], 0); err == nil && string(m[:]) == encMagic {
				return 0, false, ErrWALFormat
			}
		}
		return 0, size == 0, nil
	}
	if size < encHeaderSize {
		return 0, true, nil
	}
	h := make([]byte, encHeaderSize)
	if _, err := f.ReadAt(h, 0); err != nil {
		return 0, false, err
	}
	if string(h[:len(encMagic)]) != encMagic || binary.LittleEndian.Uint16(h[8:]) != c.keyID {
		return 0, false, ErrWALFormat
	}
	return encHeaderSize, false, nil
}

// writeHeader makes the file at path (f is its append handle) a valid empty
// encrypted log. A torn header is cut through a separate handle first: an
// append-only handle cannot truncate on Windows.
func writeHeader(f *os.File, path string, c *Cipher) error {
	if info, err := f.Stat(); err != nil {
		return err
	} else if info.Size() > 0 {
		if err := truncateFile(path, 0); err != nil {
			return err
		}
	}
	if _, err := f.Write(c.header()); err != nil {
		return err
	}
	return f.Sync()
}

// prepareFormat checks the log's format against the cipher and gives a new
// encrypted log its header.
func (w *WAL) prepareFormat(path string) error {
	_, empty, err := checkHeader(w.file, w.cipher)
	if err != nil {
		return err
	}
	if w.cipher != nil && empty {
		return writeHeader(w.file, path, w.cipher)
	}
	return nil
}

func (w *WAL) scanLog(f *os.File) (uint64, int64, error) {
	if w.cipher != nil {
		return scanLogEncrypted(f, w.cipher)
	}
	return scanLog(f)
}

func (w *WAL) scanAndTrim(f *os.File, path string, truncateCorrupt bool) (uint64, error) {
	if w.cipher != nil {
		return scanAndTrimEncrypted(f, path, w.cipher, truncateCorrupt)
	}
	return scanAndTrim(f, path, truncateCorrupt)
}

// ReplayWith is Replay for a log written with cipher c (nil: plaintext).
func ReplayWith(path string, c *Cipher, applyFn func(rec *Record) error) (RecoveryReport, error) {
	if c == nil {
		return Replay(path, applyFn)
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RecoveryReport{}, nil
		}
		return RecoveryReport{}, err
	}
	defer file.Close()

	off, empty, err := checkHeader(file, c)
	if err != nil || empty {
		return RecoveryReport{}, err
	}
	if _, err := file.Seek(off, io.SeekStart); err != nil {
		return RecoveryReport{}, err
	}
	report := RecoveryReport{}
	torn, err := c.scan(file, func(rec *Record) error {
		if err := applyFn(rec); err != nil {
			return err
		}
		report.TotalRecords++
		if rec.LSN > report.LastLSN {
			report.LastLSN = rec.LSN
		}
		return nil
	})
	report.TornWriteDetected = torn
	return report, err
}
