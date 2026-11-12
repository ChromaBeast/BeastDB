package wal

import (
	"path/filepath"
	"sync"
	"testing"
)

// Concurrent Write calls must all be durable on return, get unique monotonic
// LSNs, and replay completely after reopen.
func TestWALGroupCommitConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gc.wal")
	w, err := OpenWAL(path, true)
	if err != nil {
		t.Fatal(err)
	}
	const writers, per = 32, 25
	var wg sync.WaitGroup
	lsns := make(chan uint64, writers*per)
	for g := 0; g < writers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				lsn, err := w.Write(OpPut, []byte("k"), []byte("v"))
				if err != nil {
					t.Error(err)
					return
				}
				if w.DurableLSN() < lsn {
					t.Errorf("acknowledged LSN %d not durable (durable=%d)", lsn, w.DurableLSN())
				}
				lsns <- lsn
			}
		}()
	}
	wg.Wait()
	close(lsns)
	seen := map[uint64]bool{}
	for l := range lsns {
		if seen[l] {
			t.Fatalf("duplicate LSN %d", l)
		}
		seen[l] = true
	}
	if len(seen) != writers*per {
		t.Fatalf("got %d LSNs", len(seen))
	}
	_ = w.Close()

	var last uint64
	rep, err := Replay(path, func(r *Record) error {
		if r.LSN != last+1 {
			t.Fatalf("non-monotonic replay: %d after %d", r.LSN, last)
		}
		last = r.LSN
		return nil
	})
	if err != nil || rep.TotalRecords != writers*per {
		t.Fatalf("replay: %v total=%d", err, rep.TotalRecords)
	}
}

func TestWALAppendNotDurableUntilWait(t *testing.T) {
	w, err := OpenWAL(filepath.Join(t.TempDir(), "a.wal"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	lsn, _ := w.Append(OpPut, []byte("k"), []byte("v"))
	if w.DurableLSN() >= lsn {
		t.Fatal("Append must not mark record durable")
	}
	if err := w.WaitDurable(lsn); err != nil || w.DurableLSN() < lsn {
		t.Fatalf("WaitDurable: %v durable=%d", err, w.DurableLSN())
	}
}

func TestWALCheckpointMarksDurableAndLSNContinues(t *testing.T) {
	w, _ := OpenWAL(filepath.Join(t.TempDir(), "c.wal"), true)
	defer w.Close()
	l1, _ := w.Append(OpPut, []byte("k"), []byte("v"))
	if err := w.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	if w.DurableLSN() < l1 {
		t.Fatal("checkpoint must cover appended records")
	}
	l2, _ := w.Write(OpPut, []byte("k"), []byte("v"))
	if l2 != l1+1 {
		t.Fatalf("LSN after checkpoint: %d", l2)
	}
}

func BenchmarkWALConcurrentSyncedWrite(b *testing.B) {
	w, err := OpenWAL(filepath.Join(b.TempDir(), "b.wal"), true)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	b.SetParallelism(8)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := w.Write(OpPut, []byte("key"), []byte("value")); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
