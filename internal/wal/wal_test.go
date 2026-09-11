package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWALRecordEncodeDecode(t *testing.T) {
	orig := &Record{
		LSN:   42,
		Type:  OpPut,
		Key:   []byte("user:heisenberg"),
		Value: []byte("Walter White"),
	}

	encoded := EncodeRecord(orig)
	decoded, err := DecodeRecord(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("failed to decode record: %v", err)
	}

	if decoded.LSN != orig.LSN || decoded.Type != orig.Type {
		t.Fatalf("metadata mismatch: got LSN=%d, type=%d", decoded.LSN, decoded.Type)
	}
	if !bytes.Equal(decoded.Key, orig.Key) || !bytes.Equal(decoded.Value, orig.Value) {
		t.Fatalf("payload mismatch: key=%q, val=%q", decoded.Key, decoded.Value)
	}
}

func TestWALAppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")

	// 1. Write mutations to the WAL
	wal, err := OpenWAL(walPath, true)
	if err != nil {
		t.Fatalf("failed to open WAL: %v", err)
	}

	for i := 1; i <= 20; i++ {
		key := []byte("user:test")
		val := []byte("active")
		lsn, err := wal.Write(OpPut, key, val)
		if err != nil || lsn != uint64(i) {
			t.Fatalf("write failed: lsn=%d, err=%v", lsn, err)
		}
	}
	wal.Close()

	// 2. Simulate complete restart & replay
	recoveredMap := make(map[string]string)
	report, err := Replay(walPath, func(rec *Record) error {
		recoveredMap[string(rec.Key)] = string(rec.Value)
		return nil
	})

	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if report.TotalRecords != 20 || report.LastLSN != 20 {
		t.Fatalf("unexpected report: total=%d, lastLSN=%d", report.TotalRecords, report.LastLSN)
	}
	if report.TornWriteDetected {
		t.Fatalf("unexpected torn write reported on clean log")
	}
	if recoveredMap["user:test"] != "active" {
		t.Fatalf("recovered value mismatch: %s", recoveredMap["user:test"])
	}
}

func TestWALTornWriteRecovery(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "torn.wal")

	// 1. Write 5 records
	wal, err := OpenWAL(walPath, true)
	if err != nil {
		t.Fatalf("failed to open WAL: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, _ = wal.Write(OpPut, []byte("key"), []byte("value"))
	}
	wal.Close()

	// 2. Simulate a crash: truncate the last 7 bytes of the file (torn write!)
	info, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if err := os.Truncate(walPath, info.Size()-7); err != nil {
		t.Fatalf("failed to truncate: %v", err)
	}

	// 3. Replay must recover the 4 clean records and safely stop at record 5
	replayedCount := 0
	report, err := Replay(walPath, func(rec *Record) error {
		replayedCount++
		return nil
	})

	if err != nil {
		t.Fatalf("replay should not fail hard on torn write: %v", err)
	}
	if !report.TornWriteDetected {
		t.Fatalf("expected torn write to be detected")
	}
	if replayedCount != 4 || report.TotalRecords != 4 {
		t.Fatalf("expected 4 recovered records, got %d", report.TotalRecords)
	}
}

func BenchmarkWALAppend(b *testing.B) {
	dir := b.TempDir()
	walPath := filepath.Join(dir, "bench.wal")

	wal, err := OpenWAL(walPath, false) // un-synced for raw throughput measurement
	if err != nil {
		b.Fatalf("failed to open WAL: %v", err)
	}
	defer wal.Close()

	key := []byte("bench:key")
	val := []byte("bench:val:payload:bytes")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = wal.Write(OpPut, key, val)
	}
}
