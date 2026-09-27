package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineErrorHandling_InvalidSnapshotDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.bin")
	walPath := filepath.Join(dir, "test.wal")

	engine, err := NewEngine(dbPath, walPath, 10)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	_, err = engine.CreateSnapshot("")
	if err != ErrInvalidSnapshotDir {
		t.Fatalf("expected ErrInvalidSnapshotDir, got: %v", err)
	}
}

func TestEngineCheckpointAndClose_ErrorFreePath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.bin")
	walPath := filepath.Join(dir, "test.wal")

	engine, err := NewEngine(dbPath, walPath, 10)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if err := engine.Put(100, []byte("val-100")); err != nil {
		t.Fatalf("failed to put record: %v", err)
	}

	if err := engine.Checkpoint(); err != nil {
		t.Fatalf("checkpoint returned unexpected error: %v", err)
	}

	// Verify close completes cleanly without swallowing errors
	if err := engine.Close(); err != nil {
		t.Fatalf("close returned unexpected error: %v", err)
	}

	// Reopen and ensure state is pristine
	engine2, err := NewEngine(dbPath, walPath, 10)
	if err != nil {
		t.Fatalf("failed to reopen engine: %v", err)
	}
	defer engine2.Close()

	val, found, err := engine2.Get(100)
	if err != nil || !found || string(val) != "val-100" {
		t.Fatalf("expected val-100, got: %s (found: %v, err: %v)", val, found, err)
	}
}

func TestEngineCreateSnapshot_SafeFailureOnReadOnlyDest(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.bin")
	walPath := filepath.Join(dir, "test.wal")

	engine, err := NewEngine(dbPath, walPath, 10)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	if err := engine.Put(1, []byte("alpha")); err != nil {
		t.Fatalf("failed to put: %v", err)
	}

	// Attempting snapshot to an existing read-only file or invalid path
	invalidDest := filepath.Join(dir, "not-a-directory.txt")
	if err := os.WriteFile(invalidDest, []byte("blocker"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Passing a file path where MkdirAll should fail
	badSubdir := filepath.Join(invalidDest, "subpath")
	_, err = engine.CreateSnapshot(badSubdir)
	if err == nil {
		t.Fatalf("expected CreateSnapshot to fail on invalid directory, but succeeded")
	}
}
