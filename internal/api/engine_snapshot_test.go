package api

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestEnginePhysicalSnapshotAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "live.bin")
	walPath := filepath.Join(tempDir, "live.wal")
	backupDir := filepath.Join(tempDir, "backups")

	engine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	const totalRecords = 250
	for i := uint64(1); i <= totalRecords; i++ {
		val := []byte(fmt.Sprintf("snapshot-payload-%d", i))
		if err := engine.Put(i, val); err != nil {
			t.Fatalf("put failed for key %d: %v", i, err)
		}
	}

	// Take consistent physical snapshot
	snapshotFile, err := engine.CreateSnapshot(backupDir)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	// Continue writing to live engine after snapshot
	if err := engine.Put(9999, []byte("post-snapshot-mutation")); err != nil {
		t.Fatalf("post-snapshot put failed: %v", err)
	}
	_ = engine.Close()

	// Boot up an independent restore engine directly from the snapshot file!
	restoreWAL := filepath.Join(backupDir, "restore.wal")
	restoredEngine, err := NewEngine(snapshotFile, restoreWAL, 64)
	if err != nil {
		t.Fatalf("failed to boot engine from snapshot: %v", err)
	}
	defer restoredEngine.Close()

	// Verify all pre-snapshot records are present in restored engine
	for i := uint64(1); i <= totalRecords; i++ {
		val, found, err := restoredEngine.Get(i)
		if err != nil || !found {
			t.Fatalf("missing key %d in restored snapshot: %v", i, err)
		}
		expected := fmt.Sprintf("snapshot-payload-%d", i)
		if string(val) != expected {
			t.Fatalf("mismatched value for key %d: got %s, want %s", i, val, expected)
		}
	}

	// Verify post-snapshot mutation is NOT in the point-in-time snapshot
	_, foundPost, _ := restoredEngine.Get(9999)
	if foundPost {
		t.Fatalf("post-snapshot mutation 9999 should not exist in earlier snapshot")
	}

	// Verify that restored engine can accept new mutations
	if err := restoredEngine.Put(5555, []byte("fresh-mutation-in-restored")); err != nil {
		t.Fatalf("failed to insert into restored engine: %v", err)
	}
	val5555, found5555, err := restoredEngine.Get(5555)
	if err != nil || !found5555 || string(val5555) != "fresh-mutation-in-restored" {
		t.Fatalf("failed to retrieve new mutation from restored engine")
	}
}
