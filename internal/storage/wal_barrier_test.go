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
func TestWALBarrierFailureBlocksCheckpoint(t *testing.T) {
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
	if err := bpm.FlushAll(); !errors.Is(err, errBarrier) {
		t.Fatalf("FlushAll err = %v, want barrier error", err)
	}

	onDisk := make([]byte, PageSize)
	if _, err := disk.file.ReadAt(onDisk, int64(id*PageSize)); err == nil && bytes.Contains(onDisk, []byte("uncommitted")) {
		t.Fatal("dirty page reached the data file although the WAL barrier failed")
	}
}

// Evictions spill to scratch space and need no barrier; a checkpoint runs the
// barrier once, then installs every dirty and spilled page in the data file.
func TestWALBarrierRunsOncePerCheckpoint(t *testing.T) {
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

	// Evict via a second page (pool size 1): goes to the spill file.
	page2, id2, err := bpm.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	copy(page2.Data()[100:], []byte("second"))
	if calls != 0 {
		t.Fatalf("barrier calls after eviction = %d, want 0", calls)
	}
	if err := bpm.UnpinPage(id2, true); err != nil {
		t.Fatal(err)
	}
	if err := bpm.FlushAll(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("barrier calls after FlushAll = %d, want 1", calls)
	}

	for pid, want := range map[uint64]string{id: "committed", id2: "second"} {
		onDisk := make([]byte, PageSize)
		if _, err := disk.file.ReadAt(onDisk, int64(pid*PageSize)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(onDisk, []byte(want)) {
			t.Fatalf("page %d not in the data file after checkpoint", pid)
		}
	}
}
