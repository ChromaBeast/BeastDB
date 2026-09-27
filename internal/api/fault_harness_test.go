package api

import (
	"bytes"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
)

// TestReferenceMapModelBasedFaultHarness executes randomized mutations,
// compares state against a reference map oracle, and verifies recovery across simulated crashes.
func TestReferenceMapModelBasedFaultHarness(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "harness.bin")
	walPath := filepath.Join(tempDir, "harness.wal")

	engine, err := NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	oracle := make(map[uint64][]byte)
	rng := rand.New(rand.NewSource(1337))

	const rounds = 4
	const opsPerRound = 60

	for round := 0; round < rounds; round++ {
		for op := 0; op < opsPerRound; op++ {
			action := rng.Intn(3)
			key := uint64(rng.Intn(100) + 1)

			switch action {
			case 0: // Put
				val := []byte(fmt.Sprintf("val-r%d-o%d-%d", round, op, rng.Intn(1000)))
				if err := engine.Put(key, val); err != nil {
					t.Fatalf("put failed on key %d: %v", key, err)
				}
				oracle[key] = val

			case 1: // Delete
				if err := engine.Delete(key); err != nil {
					t.Fatalf("delete failed on key %d: %v", key, err)
				}
				delete(oracle, key)

			case 2: // BatchWrite
				batchPuts := []BatchOperation{
					{Type: BatchOpPut, Key: uint64(rng.Intn(100) + 1), Value: []byte(fmt.Sprintf("batch-p1-r%d-%d", round, op))},
					{Type: BatchOpPut, Key: uint64(rng.Intn(100) + 1), Value: []byte(fmt.Sprintf("batch-p2-r%d-%d", round, op))},
				}
				delKey := uint64(rng.Intn(100) + 1)
				batchOps := append(batchPuts, BatchOperation{Type: BatchOpDelete, Key: delKey})

				if err := engine.BatchWrite(batchOps); err != nil {
					t.Fatalf("batch write failed: %v", err)
				}
				for _, p := range batchPuts {
					oracle[p.Key] = p.Value
				}
				delete(oracle, delKey)
			}
		}

		// Verify state before crash
		verifyAgainstOracle(t, engine, oracle)

		// Simulate abrupt crash: sync WAL to ensure durability, close file handles without flushing dirty pages
		_ = engine.wal.Sync()
		_ = engine.wal.Close()
		_ = engine.disk.Close()

		// Reopen engine: crash recovery must restore full state from WAL
		reopened, err := NewEngine(dbPath, walPath, 20)
		if err != nil {
			t.Fatalf("round %d: failed to recover engine from crash: %v", round, err)
		}
		engine = reopened

		// Verify full state after recovery against reference oracle
		verifyAgainstOracle(t, engine, oracle)
	}

	_ = engine.Close()
}

func verifyAgainstOracle(t *testing.T, eng *Engine, oracle map[uint64][]byte) {
	t.Helper()

	// 1. Point lookups for all keys
	for k := uint64(1); k <= 100; k++ {
		val, found, err := eng.Get(k)
		if err != nil {
			t.Fatalf("get failed for key %d: %v", k, err)
		}

		expectedVal, existsInOracle := oracle[k]
		if existsInOracle != found {
			t.Fatalf("key %d existence mismatch: oracle=%v, db=%v", k, existsInOracle, found)
		}
		if found && !bytes.Equal(val, expectedVal) {
			t.Fatalf("key %d value mismatch: expected %q, got %q", k, expectedVal, val)
		}
	}

	// 2. Range scan validation across entire key range
	cursor, err := eng.Scan(1, 100)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	defer cursor.Close()

	var scannedKeys []uint64
	for {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			t.Fatalf("cursor next failed: %v", err)
		}
		if !ok {
			break
		}
		val, err := eng.ReadTuple(rid)
		if err != nil {
			t.Fatalf("read tuple failed for key %d: %v", key, err)
		}
		scannedKeys = append(scannedKeys, key)
		expected, exists := oracle[key]
		if !exists {
			t.Fatalf("scan returned deleted or phantom key %d", key)
		}
		if !bytes.Equal(val, expected) {
			t.Fatalf("scan value mismatch for key %d", key)
		}
	}

	var expectedSortedKeys []uint64
	for k := range oracle {
		if k >= 1 && k <= 100 {
			expectedSortedKeys = append(expectedSortedKeys, k)
		}
	}
	sort.Slice(expectedSortedKeys, func(i, j int) bool { return expectedSortedKeys[i] < expectedSortedKeys[j] })

	if len(scannedKeys) != len(expectedSortedKeys) {
		t.Fatalf("scan count mismatch: expected %d, got %d", len(expectedSortedKeys), len(scannedKeys))
	}
	for i := range scannedKeys {
		if scannedKeys[i] != expectedSortedKeys[i] {
			t.Fatalf("scan order mismatch at index %d: expected %d, got %d", i, expectedSortedKeys[i], scannedKeys[i])
		}
	}
}
