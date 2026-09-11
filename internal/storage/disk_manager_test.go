package storage

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestDiskManager(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_disk.bin")

	disk, err := OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk manager: %v", err)
	}
	defer disk.Close()

	if disk.NumPages() != 0 {
		t.Fatalf("expected 0 pages initially, got %d", disk.NumPages())
	}

	p0, err := disk.AllocatePage()
	if err != nil || p0 != 0 {
		t.Fatalf("unexpected allocate p0: %d, err: %v", p0, err)
	}

	p1, err := disk.AllocatePage()
	if err != nil || p1 != 1 {
		t.Fatalf("unexpected allocate p1: %d, err: %v", p1, err)
	}

	if disk.NumPages() != 2 {
		t.Fatalf("expected 2 pages, got %d", disk.NumPages())
	}

	writeBuf := make([]byte, PageSize)
	copy(writeBuf, "hello disk manager page 0")
	if err := disk.WritePage(0, writeBuf); err != nil {
		t.Fatalf("failed to write page 0: %v", err)
	}

	readBuf := make([]byte, PageSize)
	if err := disk.ReadPage(0, readBuf); err != nil {
		t.Fatalf("failed to read page 0: %v", err)
	}

	if !bytes.Equal(readBuf[:25], []byte("hello disk manager page 0")) {
		t.Fatalf("read content mismatch: %s", string(readBuf[:25]))
	}

	if err := disk.Close(); err != nil {
		t.Fatalf("failed to close disk manager: %v", err)
	}

	reopened, err := OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen disk manager: %v", err)
	}
	defer reopened.Close()

	if reopened.NumPages() != 2 {
		t.Fatalf("expected 2 pages after reopening, got %d", reopened.NumPages())
	}

	readReopen := make([]byte, PageSize)
	if err := reopened.ReadPage(0, readReopen); err != nil {
		t.Fatalf("failed to read page 0 after reopen: %v", err)
	}
	if !bytes.Equal(readReopen[:25], []byte("hello disk manager page 0")) {
		t.Fatalf("reopened content mismatch")
	}
}
