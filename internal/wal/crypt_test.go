package wal

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testWALCipher(t *testing.T) *Cipher {
	t.Helper()
	block, _ := aes.NewCipher(make([]byte, 32))
	aead, _ := cipher.NewGCM(block)
	return NewCipher(aead, 1)
}

func writeLog(t *testing.T, path string, c *Cipher, n int) {
	t.Helper()
	w, err := OpenWALCipher(path, true, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := w.Write(OpPut, []byte{byte(i), 0, 0, 0, 0, 0, 0, 0}, []byte("encrypted-value-payload")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedWALReplayReopenAndRotate(t *testing.T) {
	c := testWALCipher(t)
	path := filepath.Join(t.TempDir(), "x.wal")
	writeLog(t, path, c, 5)
	raw, _ := os.ReadFile(path)
	if string(raw[:8]) != encMagic || contains(raw, []byte("encrypted-value-payload")) {
		t.Fatal("WAL is not encrypted")
	}
	var lsns []uint64
	rep, err := ReplayWith(path, c, func(r *Record) error { lsns = append(lsns, r.LSN); return nil })
	if err != nil || rep.TotalRecords != 5 || rep.LastLSN != 5 || rep.TornWriteDetected {
		t.Fatalf("replay: %+v %v", rep, err)
	}
	w, err := OpenWALCipher(path, true, c) // LSNs continue after reopen
	if err != nil {
		t.Fatal(err)
	}
	if lsn, _ := w.Write(OpPut, make([]byte, 8), nil); lsn != 6 {
		t.Fatalf("next LSN %d", lsn)
	}
	if err := w.Checkpoint(); err != nil { // rotation writes a fresh header
		t.Fatal(err)
	}
	if lsn, _ := w.Write(OpPut, make([]byte, 8), nil); lsn != 7 {
		t.Fatalf("LSN after rotation %d", lsn)
	}
	_ = w.Close()
	if rep, err := ReplayWith(path, c, func(*Record) error { return nil }); err != nil || rep.TotalRecords != 1 {
		t.Fatalf("after rotation: %+v %v", rep, err)
	}
	if rep, err := ReplayWith(path+".bak", c, func(*Record) error { return nil }); err != nil || rep.TotalRecords != 6 {
		t.Fatalf("archived generation: %+v %v", rep, err)
	}
}

func contains(b, sub []byte) bool {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == string(sub) {
			return true
		}
	}
	return false
}

func TestEncryptedWALTornTailVersusTamper(t *testing.T) {
	c := testWALCipher(t)
	dir := t.TempDir()

	torn := filepath.Join(dir, "torn.wal")
	writeLog(t, torn, c, 4)
	info, _ := os.Stat(torn)
	if err := os.Truncate(torn, info.Size()-10); err != nil { // cut the last frame short
		t.Fatal(err)
	}
	if rep, err := ReplayWith(torn, c, func(*Record) error { return nil }); err != nil || rep.TotalRecords != 3 || !rep.TornWriteDetected {
		t.Fatalf("torn tail: %+v %v", rep, err)
	}

	// Garbage in the last frame (full length, bad tag) with nothing after it: torn.
	last := filepath.Join(dir, "last.wal")
	writeLog(t, last, c, 4)
	info, _ = os.Stat(last)
	f, _ := os.OpenFile(last, os.O_RDWR, 0)
	_, _ = f.WriteAt([]byte{0xFF}, info.Size()-5)
	_ = f.Close()
	if rep, err := ReplayWith(last, c, func(*Record) error { return nil }); err != nil || rep.TotalRecords != 3 || !rep.TornWriteDetected {
		t.Fatalf("bad last frame: %+v %v", rep, err)
	}

	// Bad tag in an early frame with valid frames after it: tampering.
	mid := filepath.Join(dir, "mid.wal")
	writeLog(t, mid, c, 4)
	f, _ = os.OpenFile(mid, os.O_RDWR, 0)
	_, _ = f.WriteAt([]byte{0xFF}, encHeaderSize+frameClear+12+3)
	_ = f.Close()
	if _, err := ReplayWith(mid, c, func(*Record) error { return nil }); !errors.Is(err, ErrWALTampered) {
		t.Fatalf("mid-log tamper: want ErrWALTampered, got %v", err)
	}
	if _, err := OpenWALCipher(mid, true, c); !errors.Is(err, ErrWALTampered) {
		t.Fatalf("open of tampered log: want ErrWALTampered, got %v", err)
	}

	// A frame moved to another LSN (clear LSN edited) fails authentication.
	lsn := filepath.Join(dir, "lsn.wal")
	writeLog(t, lsn, c, 4)
	f, _ = os.OpenFile(lsn, os.O_RDWR, 0)
	_, _ = f.WriteAt([]byte{0x63}, encHeaderSize+4) // first frame's LSN byte
	_ = f.Close()
	if _, err := ReplayWith(lsn, c, func(*Record) error { return nil }); !errors.Is(err, ErrWALTampered) {
		t.Fatalf("edited LSN: want ErrWALTampered, got %v", err)
	}
}

func TestWrongWALKeyIsNotMistakenForData(t *testing.T) {
	c := testWALCipher(t)
	path := filepath.Join(t.TempDir(), "x.wal")
	writeLog(t, path, c, 3)
	block, _ := aes.NewCipher(append(make([]byte, 31), 1))
	aead, _ := cipher.NewGCM(block)
	other := NewCipher(aead, 1)
	rep, err := ReplayWith(path, other, func(*Record) error { return nil })
	if err != nil || rep.TotalRecords != 0 {
		t.Fatalf("wrong key replayed records: %+v %v", rep, err)
	}
}
