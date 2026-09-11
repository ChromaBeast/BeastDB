package storage

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBufferPoolBasicAndEviction(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bpm.bin")

	disk, err := OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}
	defer disk.Close()

	// Pool of size 2
	bpm := NewBufferPoolManager(disk, 2)

	// Allocate page 0
	p0, id0, err := bpm.NewPage()
	if err != nil || id0 != 0 {
		t.Fatalf("failed to allocate page 0: %v", err)
	}
	s0, _ := p0.InsertTuple([]byte("record in page 0"))
	if err := bpm.UnpinPage(id0, true); err != nil {
		t.Fatalf("failed to unpin page 0: %v", err)
	}

	// Allocate page 1
	p1, id1, err := bpm.NewPage()
	if err != nil || id1 != 1 {
		t.Fatalf("failed to allocate page 1: %v", err)
	}
	s1, _ := p1.InsertTuple([]byte("record in page 1"))
	if err := bpm.UnpinPage(id1, true); err != nil {
		t.Fatalf("failed to unpin page 1: %v", err)
	}

	// Allocate page 2 -> triggers eviction of page 0 (clock hand sweeps)
	p2, id2, err := bpm.NewPage()
	if err != nil || id2 != 2 {
		t.Fatalf("failed to allocate page 2: %v", err)
	}
	p2.InsertTuple([]byte("record in page 2"))
	if err := bpm.UnpinPage(id2, true); err != nil {
		t.Fatalf("failed to unpin page 2: %v", err)
	}

	// Fetch evicted page 0 back from disk
	p0Reloaded, err := bpm.FetchPage(id0)
	if err != nil {
		t.Fatalf("failed to reload evicted page 0: %v", err)
	}
	val0, err := p0Reloaded.GetTuple(s0)
	if err != nil || !bytes.Equal(val0, []byte("record in page 0")) {
		t.Fatalf("reloaded data corrupt: got %s, err: %v", string(val0), err)
	}
	if err := bpm.UnpinPage(id0, false); err != nil {
		t.Fatalf("failed to unpin reloaded page 0: %v", err)
	}

	// Fetch page 1 (might also be evicted or in pool)
	p1Reloaded, err := bpm.FetchPage(id1)
	if err != nil {
		t.Fatalf("failed to reload page 1: %v", err)
	}
	val1, err := p1Reloaded.GetTuple(s1)
	if err != nil || !bytes.Equal(val1, []byte("record in page 1")) {
		t.Fatalf("reloaded data corrupt: got %s, err: %v", string(val1), err)
	}
	if err := bpm.UnpinPage(id1, false); err != nil {
		t.Fatalf("failed to unpin reloaded page 1: %v", err)
	}
}

func TestBufferPoolPinningProtection(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bpm_pins.bin")

	disk, err := OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}
	defer disk.Close()

	// Pool of size 2
	bpm := NewBufferPoolManager(disk, 2)

	// Allocate 2 pages and keep them PINNED
	_, id0, err := bpm.NewPage()
	if err != nil {
		t.Fatalf("failed to allocate page 0: %v", err)
	}
	_, id1, err := bpm.NewPage()
	if err != nil {
		t.Fatalf("failed to allocate page 1: %v", err)
	}

	// Attempting to allocate 3rd page while all frames are pinned must fail
	_, _, err = bpm.NewPage()
	if err != ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}

	// Unpin page 0
	if err := bpm.UnpinPage(id0, false); err != nil {
		t.Fatalf("failed to unpin page 0: %v", err)
	}

	// Now allocation should succeed by evicting page 0
	_, id2, err := bpm.NewPage()
	if err != nil || id2 != 2 {
		t.Fatalf("expected successful allocation after unpinning, got err %v", err)
	}

	// Unpin remaining pages
	_ = bpm.UnpinPage(id1, false)
	_ = bpm.UnpinPage(id2, false)
}

func TestBufferPoolUnpinErrors(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bpm_errs.bin")

	disk, err := OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}
	defer disk.Close()

	bpm := NewBufferPoolManager(disk, 1)

	// Unpin non-existent page
	if err := bpm.UnpinPage(999, false); err != ErrPageNotFound {
		t.Fatalf("expected ErrPageNotFound, got %v", err)
	}

	_, id0, _ := bpm.NewPage()
	if err := bpm.UnpinPage(id0, false); err != nil {
		t.Fatalf("unexpected unpin error: %v", err)
	}

	// Unpin already unpinned page
	if err := bpm.UnpinPage(id0, false); err != ErrPageNotPinned {
		t.Fatalf("expected ErrPageNotPinned, got %v", err)
	}
}
