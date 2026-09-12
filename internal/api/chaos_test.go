package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/wal"
)

func TestChaosTornWriteRecovery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "chaos_test.bin")
	walPath := filepath.Join(tempDir, "chaos_test.wal")

	engine, err := NewEngine(dbPath, walPath, 50)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. Commit 50 valid records
	for i := uint64(1); i <= 50; i++ {
		val := []byte(fmt.Sprintf("stable-value-%d", i))
		if err := engine.Put(i, val); err != nil {
			t.Fatalf("put failed for key %d: %v", i, err)
		}
	}

	// 2. Cleanly close to simulate disk state before sudden power event
	if err := engine.Close(); err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	// 3. Chaos Fault Injection: Append corrupted/torn write to the tail of the WAL
	walFile, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open wal file: %v", err)
	}
	// Write an incomplete 15-byte corrupted header (simulating abrupt power cut mid-write)
	corruptedTail := []byte{0x00, 0x00, 0x00, 0x2A, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	if _, err := walFile.Write(corruptedTail); err != nil {
		t.Fatalf("failed to write corrupted tail: %v", err)
	}
	_ = walFile.Sync()
	_ = walFile.Close()

	// 4. Verify that WAL recovery detects the torn write safely
	replayedCount := 0
	report, err := wal.Replay(walPath, func(rec *wal.Record) error {
		replayedCount++
		return nil
	})
	if err != nil {
		t.Fatalf("wal replay failed unexpectedly: %v", err)
	}
	if !report.TornWriteDetected {
		t.Fatalf("expected torn write detection to be true")
	}
	if replayedCount != 50 {
		t.Fatalf("expected 50 valid records replayed, got %d", replayedCount)
	}

	// 5. Reopen database engine and verify all valid records are preserved
	reopened, err := NewEngine(dbPath, walPath, 50)
	if err != nil {
		t.Fatalf("failed to reopen engine after torn write: %v", err)
	}
	defer reopened.Close()

	for i := uint64(1); i <= 50; i++ {
		expected := []byte(fmt.Sprintf("stable-value-%d", i))
		val, found, err := reopened.Get(i)
		if err != nil || !found || !bytes.Equal(val, expected) {
			t.Fatalf("key %d verification failed: found=%v, err=%v", i, found, err)
		}
	}

	// 6. Verify that new writes continue to succeed cleanly after recovery
	newKey := uint64(9999)
	newVal := []byte("post-recovery-write")
	if err := reopened.Put(newKey, newVal); err != nil {
		t.Fatalf("failed to write new record after recovery: %v", err)
	}
	gotVal, found, err := reopened.Get(newKey)
	if err != nil || !found || !bytes.Equal(gotVal, newVal) {
		t.Fatalf("failed to get post-recovery key: %v", err)
	}
}

func TestConcurrentStress(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "stress_test.bin")
	walPath := filepath.Join(tempDir, "stress_test.wal")

	engine, err := NewEngine(dbPath, walPath, 100)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	const numWorkers = 20
	const opsPerWorker = 100
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		workerID := uint64(w)
		go func() {
			defer wg.Done()
			for i := uint64(0); i < opsPerWorker; i++ {
				key := workerID*1000 + i
				val := []byte(fmt.Sprintf("worker-%d-op-%d", workerID, i))

				// Mixed workload: Put -> Get -> Scan
				_ = engine.Put(key, val)
				_, _, _ = engine.Get(key)

				if i%20 == 0 {
					cursor, err := engine.Scan(workerID*1000, key)
					if err == nil {
						for {
							_, _, ok, _ := cursor.Next()
							if !ok {
								break
							}
						}
						_ = cursor.Close()
					}
				}
			}
		}()
	}

	wg.Wait()
}
