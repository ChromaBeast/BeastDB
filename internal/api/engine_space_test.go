package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOverwriteAndDeleteReuseSpace(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEngine(filepath.Join(dir, "d.bin"), filepath.Join(dir, "d.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	val := make([]byte, 500)
	for round := 0; round < 3; round++ {
		for k := uint64(1); k <= 100; k++ {
			if err := e.Put(k, val); err != nil {
				t.Fatal(err)
			}
		}
	}
	pagesAfter3 := e.disk.NumPages()
	for round := 0; round < 20; round++ {
		for k := uint64(1); k <= 100; k++ {
			if err := e.Put(k, val); err != nil {
				t.Fatal(err)
			}
		}
	}
	if grew := e.disk.NumPages() - pagesAfter3; grew > 4 {
		t.Fatalf("file grew by %d pages across overwrites; dead tuples not reclaimed", grew)
	}
	for k := uint64(1); k <= 100; k++ {
		if v, ok, err := e.Get(k); err != nil || !ok || len(v) != 500 {
			t.Fatalf("key %d unreadable after reuse: ok=%v err=%v", k, ok, err)
		}
	}
}

func TestHaltBlocksWritesAfterApplyFailure(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEngine(filepath.Join(dir, "h.bin"), filepath.Join(dir, "h.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.halt(fmt.Errorf("injected"))
	e.mu.Unlock()
	if err := e.Put(2, []byte("b")); err == nil {
		t.Fatal("write succeeded on halted engine")
	}
	if err := e.BatchWrite([]BatchOperation{{Type: BatchOpPut, Key: 3, Value: []byte("c")}}); err == nil {
		t.Fatal("batch succeeded on halted engine")
	}
	if _, ok, _ := e.Get(1); !ok {
		t.Fatal("reads must keep working while halted")
	}
}

// Replaying a WAL tail whose effects were already flushed must not duplicate or orphan data.
func TestReplayOfAlreadyFlushedTailIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, wl := filepath.Join(dir, "r.bin"), filepath.Join(dir, "r.wal")
	e, err := NewEngine(db, wl, 32)
	if err != nil {
		t.Fatal(err)
	}
	for k := uint64(1); k <= 50; k++ {
		_ = e.Put(k, []byte(fmt.Sprintf("v%d", k)))
	}
	_ = e.Delete(7)
	// Flush pages but pretend the checkpoint LSN was never recorded: crash after flush.
	if err := e.bpm.FlushAll(); err != nil {
		t.Fatal(err)
	}
	walCopy, _ := os.ReadFile(wl)
	_ = e.Close()
	_ = os.WriteFile(wl, walCopy, 0600)

	// Reset the checkpoint LSN to 0 so the whole WAL replays over flushed pages.
	e2, err := NewEngine(db, wl, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	for k := uint64(1); k <= 50; k++ {
		v, ok, err := e2.Get(k)
		if k == 7 {
			if ok {
				t.Fatal("deleted key resurrected")
			}
			continue
		}
		if err != nil || !ok || string(v) != fmt.Sprintf("v%d", k) {
			t.Fatalf("key %d: %q ok=%v err=%v", k, v, ok, err)
		}
	}
}

func TestAutoCheckpointRotatesWAL(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEngine(filepath.Join(dir, "c.bin"), filepath.Join(dir, "c.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for k := uint64(1); k <= 200; k++ {
		_ = e.Put(k, make([]byte, 100))
	}
	before := e.wal.FileSize()
	if err := e.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	if after := e.wal.FileSize(); after >= before {
		t.Fatalf("WAL not rotated: %d -> %d", before, after)
	}
}
