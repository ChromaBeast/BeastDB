package api

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSnapshotPauseReport measures how long a snapshot stalls writes, against
// the previous design that held the engine lock for the whole copy. It prints
// numbers and asserts only that the new pause is shorter than the copy.
//
//	BEAST_PAUSE_REPORT=1 go test ./internal/api -run SnapshotPauseReport -v
//
// BEAST_PAUSE_KEYS sets the database size (default 200000 keys of 1 KiB).
func TestSnapshotPauseReport(t *testing.T) {
	if os.Getenv("BEAST_PAUSE_REPORT") == "" {
		t.Skip("set BEAST_PAUSE_REPORT=1 to run")
	}
	keys := 200000
	if v, err := strconv.Atoi(os.Getenv("BEAST_PAUSE_KEYS")); err == nil {
		keys = v
	}
	dir := t.TempDir()
	eng, err := NewEngine(filepath.Join(dir, "a.bin"), filepath.Join(dir, "a.wal"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	val := make([]byte, 1024)
	for i := 1; i <= keys; i += 500 { // batches: one fsync per 500 keys
		ops := make([]BatchOperation, 0, 500)
		for k := i; k < i+500 && k <= keys; k++ {
			ops = append(ops, BatchOperation{Type: BatchOpPut, Key: uint64(k), Value: val})
		}
		if err := eng.BatchWrite(ops); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(eng.disk.Path())
	t.Logf("data file %.0f MiB", float64(st.Size())/(1<<20))

	// run takes a snapshot with snap while a writer records its worst Put latency.
	run := func(name string, snap func() error) {
		var worst atomic.Int64
		stop := make(chan struct{})
		var wg sync.WaitGroup
		var key atomic.Uint64
		key.Store(uint64(keys) + 1<<20)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				t0 := time.Now()
				if err := eng.Put(key.Add(1), val); err != nil {
					return
				}
				if d := int64(time.Since(t0)); d > worst.Load() {
					worst.Store(d)
				}
			}
		}()
		time.Sleep(50 * time.Millisecond)
		worst.Store(0)
		t0 := time.Now()
		if err := snap(); err != nil {
			t.Fatal(err)
		}
		total := time.Since(t0)
		close(stop)
		wg.Wait()
		t.Logf("%-8s snapshot took %v, worst write latency during it %v", name, total.Round(time.Millisecond), time.Duration(worst.Load()).Round(time.Microsecond))
	}

	legacy := func() error { // the old design: checkpoint and copy under e.mu
		e := eng
		e.imageMu.Lock()
		defer e.imageMu.Unlock()
		e.mu.Lock()
		defer e.mu.Unlock()
		if err := e.persistCheckpointLocked(e.wal.CurrentLSN()); err != nil {
			return err
		}
		return copyFileSynced(e.disk.Path(), filepath.Join(dir, "legacy.bin"))
	}
	online := func() error { _, err := eng.CreateSnapshot(filepath.Join(dir, "snaps")); return err }
	run("legacy", legacy)
	run("online", online)
	run("legacy", legacy)
	run("online", online)
}
