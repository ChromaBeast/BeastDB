package storage

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

func newBarrierPool(t *testing.T, size int) (*BufferPoolManager, *DiskManager) {
	t.Helper()
	disk, err := OpenDiskManager(filepath.Join(t.TempDir(), "barrier.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	return NewBufferPoolManager(disk, size), disk
}

// A dirty page must not reach the data file when the WAL barrier fails:
// otherwise a page could be persisted ahead of the log records that describe it.
func TestWALBarrierFailureBlocksPageWrite(t *testing.T) {
	bpm, disk := newBarrierPool(t, 1)
	errBarrier := errors.New("wal not durable")
	bpm.SetWALBarrier(func() error { return errBarrier })

	page, id, err := bpm.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	copy(page.Data()[100:], []byte("uncommitted"))
	if err := bpm.UnpinPage(id, true); err != nil {
		t.Fatal(err)
	}

	// Eviction (pool of one) must fail because the barrier fails.
	if _, _, err := bpm.NewPage(); !errors.Is(err, errBarrier) {
		t.Fatalf("eviction err = %v, want barrier error", err)
	}
	if err := bpm.FlushPage(id); !errors.Is(err, errBarrier) {
		t.Fatalf("FlushPage err = %v, want barrier error", err)
	}
	if err := bpm.FlushAll(); !errors.Is(err, errBarrier) {
		t.Fatalf("FlushAll err = %v, want barrier error", err)
	}

	onDisk := make([]byte, PageSize)
	if err := disk.ReadPage(id, onDisk); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte("uncommitted")) {
		t.Fatal("dirty page reached the data file although the WAL barrier failed")
	}
}

// The barrier runs before every page write path, and pages are written after it.
func TestWALBarrierRunsBeforeEveryPageWrite(t *testing.T) {
	bpm, disk := newBarrierPool(t, 1)
	calls := 0
	bpm.SetWALBarrier(func() error { calls++; return nil })

	page, id, err := bpm.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	copy(page.Data()[100:], []byte("committed"))
	if err := bpm.UnpinPage(id, true); err != nil {
		t.Fatal(err)
	}

	// Evict via a second page (pool size 1).
	_, id2, err := bpm.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("barrier calls after eviction = %d, want 1", calls)
	}
	onDisk := make([]byte, PageSize)
	if err := disk.ReadPage(id, onDisk); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(onDisk, []byte("committed")) {
		t.Fatal("evicted page was not written after the barrier succeeded")
	}

	if err := bpm.UnpinPage(id2, true); err != nil {
		t.Fatal(err)
	}
	if err := bpm.FlushPage(id2); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("barrier calls after FlushPage = %d, want 2", calls)
	}

	// Dirty again, then FlushAll.
	p, err := bpm.FetchPage(id2)
	if err != nil {
		t.Fatal(err)
	}
	copy(p.Data()[200:], []byte("more"))
	if err := bpm.UnpinPage(id2, true); err != nil {
		t.Fatal(err)
	}
	if err := bpm.FlushAll(); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("barrier calls after FlushAll = %d, want 3", calls)
	}
}
