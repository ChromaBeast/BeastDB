package api

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestEngineRootPersistenceAcrossSplits(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "root_split_test.bin")
	walPath := filepath.Join(tempDir, "root_split_test.wal")

	// 1. Initialize engine and insert 500 keys to force B+ Tree root split
	engine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	const totalKeys = 500
	for i := uint64(1); i <= totalKeys; i++ {
		val := []byte(fmt.Sprintf("split-value-%d", i))
		if err := engine.Put(i, val); err != nil {
			t.Fatalf("put failed for key %d: %v", i, err)
		}
	}

	initialRootID := engine.tree.RootPageID()
	// Meta page is 0, initial root was 1. After splits, root should be > 1.
	if initialRootID <= 1 {
		t.Fatalf("expected tree root to have migrated past page 1 after splits, got %d", initialRootID)
	}

	// 2. Cleanly close engine
	if err := engine.Close(); err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	// 3. Reopen engine from disk — it must restore the correct migrated root ID from Page 0 Meta
	reopened, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to reopen engine: %v", err)
	}
	defer reopened.Close()

	if reopened.tree.RootPageID() != initialRootID {
		t.Fatalf("expected reopened root ID %d to match original root ID %d", reopened.tree.RootPageID(), initialRootID)
	}

	// 4. Verify all 500 keys are retrievable
	for i := uint64(1); i <= totalKeys; i++ {
		expected := []byte(fmt.Sprintf("split-value-%d", i))
		val, found, err := reopened.Get(i)
		if err != nil || !found {
			t.Fatalf("failed to find key %d after reopen: found=%v, err=%v", i, found, err)
		}
		if string(val) != string(expected) {
			t.Fatalf("mismatched value for key %d: got %s, want %s", i, val, expected)
		}
	}

	// 5. Verify ordered range scan across migrated tree structure
	cursor, err := reopened.Scan(1, totalKeys)
	if err != nil {
		t.Fatalf("scan failed after reopen: %v", err)
	}
	defer cursor.Close()

	scannedCount := 0
	for {
		_, _, ok, err := cursor.Next()
		if err != nil {
			t.Fatalf("scan error: %v", err)
		}
		if !ok {
			break
		}
		scannedCount++
	}

	if scannedCount != totalKeys {
		t.Fatalf("expected %d scanned keys, got %d", totalKeys, scannedCount)
	}
}

func TestEngineCrashRecoveryFromWAL(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "crash_recovery.bin")
	walPath := filepath.Join(tempDir, "crash_recovery.wal")

	engine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. Commit 100 records
	const totalRecords = 100
	for i := uint64(1); i <= totalRecords; i++ {
		val := []byte(fmt.Sprintf("crash-val-%d", i))
		if err := engine.Put(i, val); err != nil {
			t.Fatalf("put failed for key %d: %v", i, err)
		}
	}

	// 2. Simulate abrupt process kill without engine.Close() or bpm.FlushAll()
	// Sync the WAL to simulate durability, then close underlying files abruptly
	_ = engine.wal.Sync()
	_ = engine.wal.Close()
	_ = engine.disk.Close()

	// 3. Reopen engine — NewEngine must replay the WAL to reconstruct state
	recoveredEngine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to reopen engine for recovery: %v", err)
	}
	defer recoveredEngine.Close()

	// 4. Verify all 100 records were recovered from the WAL
	for i := uint64(1); i <= totalRecords; i++ {
		expected := []byte(fmt.Sprintf("crash-val-%d", i))
		val, found, err := recoveredEngine.Get(i)
		if err != nil || !found {
			t.Fatalf("missing key %d after crash recovery: found=%v, err=%v", i, found, err)
		}
		if string(val) != string(expected) {
			t.Fatalf("recovered value mismatch for key %d: got %s, want %s", i, val, expected)
		}
	}
}
