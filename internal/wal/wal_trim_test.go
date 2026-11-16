package wal

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func appendBytes(t *testing.T, path string, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func replayLSNs(t *testing.T, path string) []uint64 {
	t.Helper()
	var lsns []uint64
	if _, err := Replay(path, func(r *Record) error { lsns = append(lsns, r.LSN); return nil }); err != nil {
		t.Fatal(err)
	}
	return lsns
}

func writeThree(t *testing.T, path string) {
	t.Helper()
	w, err := OpenWAL(path, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := w.Write(OpPut, []byte("k"), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
}

// Records written after recovering from a torn tail must be reachable by the
// next replay: the torn bytes have to be cut off, not appended behind.
func TestOpenTrimsTornTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.wal")
	writeThree(t, path)
	torn := EncodeRecord(&Record{LSN: 4, Type: OpPut, Key: []byte("k"), Value: []byte("partial-payload")})
	appendBytes(t, path, torn[:len(torn)-5])

	w, err := OpenWAL(path, false)
	if err != nil {
		t.Fatal(err)
	}
	lsn, err := w.Write(OpPut, []byte("k"), []byte("after"))
	if err != nil || lsn != 4 {
		t.Fatalf("write after torn tail: lsn=%d err=%v", lsn, err)
	}
	w.Close()

	if got := replayLSNs(t, path); len(got) != 4 || got[3] != 4 {
		t.Fatalf("replay after torn-tail recovery = %v, want LSNs 1..4", got)
	}
	if _, err := os.Stat(path + ".discarded"); err == nil {
		t.Fatalf("a plain torn tail should not be saved aside")
	}
}

func TestOpenAcceptsZeroFilledTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.wal")
	writeThree(t, path)
	appendBytes(t, path, make([]byte, 4096))

	w, err := OpenWAL(path, false)
	if err != nil {
		t.Fatalf("zero-filled tail must not prevent opening: %v", err)
	}
	if w.CurrentLSN() != 3 {
		t.Fatalf("LSN = %d, want 3", w.CurrentLSN())
	}
	w.Close()
	if info, _ := os.Stat(path); info.Size()%4096 == 0 && info.Size() >= 4096 {
		t.Fatalf("zero tail was not trimmed (size %d)", info.Size())
	}
}

// Bytes after a bad record that are not a torn tail are kept in a side file.
func TestOpenSavesNonTailBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.wal")
	writeThree(t, path)
	data, _ := os.ReadFile(path)
	recLen := int(binary.LittleEndian.Uint32(data[0:4]))
	data[recLen+HeaderSize] ^= 0xff // damage record 2's payload; record 3 stays intact
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	w, err := OpenWAL(path, false)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	saved, err := os.ReadFile(path + ".discarded")
	if err != nil {
		t.Fatalf("expected discarded bytes to be saved: %v", err)
	}
	if !bytes.Equal(saved, data[recLen:]) {
		t.Fatalf("saved %d bytes, want the %d bytes after the last valid record", len(saved), len(data)-recLen)
	}
}

func TestHeaderCorruptionDetected(t *testing.T) {
	enc := EncodeRecord(&Record{LSN: 7, Type: OpPut, Key: []byte("k"), Value: []byte("v")})
	enc[4] ^= 0x01 // flip a bit in the LSN
	if _, err := DecodeRecord(bytes.NewReader(enc)); err != ErrTornWrite {
		t.Fatalf("corrupt LSN: got %v, want ErrTornWrite", err)
	}
}

// Records written before the header was checksummed must still decode.
func TestLegacyPayloadCRCRecordDecodes(t *testing.T) {
	enc := EncodeRecord(&Record{LSN: 9, Type: OpDelete, Key: []byte("k"), Value: []byte("v")})
	enc[12] = OpDelete // no header flag
	binary.LittleEndian.PutUint32(enc[19:23], crc32.ChecksumIEEE(enc[HeaderSize:]))
	rec, err := DecodeRecord(bytes.NewReader(enc))
	if err != nil || rec.LSN != 9 || rec.Type != OpDelete {
		t.Fatalf("legacy record: %+v, %v", rec, err)
	}
}
