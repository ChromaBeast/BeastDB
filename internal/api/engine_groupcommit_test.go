package api

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

type recObs struct {
	mu   sync.Mutex
	lsns []uint64
	eng  *Engine
}

func (o *recObs) OnCommit(lsn uint64, _ byte, _, _ []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	// The observer must only ever see records that are already durable.
	if o.eng.wal.DurableLSN() < lsn {
		panic(fmt.Sprintf("observer saw LSN %d before durable (%d)", lsn, o.eng.wal.DurableLSN()))
	}
	o.lsns = append(o.lsns, lsn)
}

// Many concurrent writers of mixed kinds; observer sees strictly increasing,
// durable LSNs; after an abrupt crash (no Close/flush) every acknowledged
// write is recovered.
func TestGroupCommitConcurrentWritersCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath, walPath := filepath.Join(dir, "g.bin"), filepath.Join(dir, "g.wal")
	e, err := NewEngine(dbPath, walPath, 256)
	if err != nil {
		t.Fatal(err)
	}
	obs := &recObs{eng: e}
	e.SetCommitObserver(obs)

	const writers, per = 16, 40
	var wg sync.WaitGroup
	for g := 0; g < writers; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				k := uint64(g*1000 + i + 1)
				val := []byte(fmt.Sprintf("v-%d", k))
				var err error
				switch i % 4 {
				case 0:
					err = e.Put(k, val)
				case 1:
					err = e.PutIfAbsent(k, val)
				case 2:
					err = e.BatchWrite([]BatchOperation{{Type: BatchOpPut, Key: k, Value: val}})
				case 3:
					err = e.PutIf(k, val, Condition{Kind: CondAbsent})
				}
				if err != nil {
					t.Errorf("write %d: %v", k, err)
				}
			}
		}(g)
	}
	wg.Wait()

	obs.mu.Lock()
	if len(obs.lsns) != writers*per {
		t.Fatalf("observer got %d events, want %d", len(obs.lsns), writers*per)
	}
	for i := 1; i < len(obs.lsns); i++ {
		if obs.lsns[i] != obs.lsns[i-1]+1 {
			t.Fatalf("observer order broken at %d: %d then %d", i, obs.lsns[i-1], obs.lsns[i])
		}
	}
	obs.mu.Unlock()

	// Simulate crash: stop the committer, drop files without flushing pages.
	e.stopCommitter()
	_ = e.wal.Close()
	_ = e.disk.Close()

	r, err := NewEngine(dbPath, walPath, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for g := 0; g < writers; g++ {
		for i := 0; i < per; i++ {
			k := uint64(g*1000 + i + 1)
			v, ok, err := r.Get(k)
			if err != nil || !ok || string(v) != fmt.Sprintf("v-%d", k) {
				t.Fatalf("key %d lost after crash: ok=%v err=%v v=%q", k, ok, err, v)
			}
		}
	}
}

// Conditional writes stay atomic under contention: exactly one PutIfAbsent wins.
func TestGroupCommitConditionalAtomicity(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEngine(filepath.Join(dir, "a.bin"), filepath.Join(dir, "a.wal"), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.PutIfAbsent(7, []byte("x")) == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
}

func TestWriteAfterCloseFails(t *testing.T) {
	dir := t.TempDir()
	e, _ := NewEngine(filepath.Join(dir, "c.bin"), filepath.Join(dir, "c.wal"), 64)
	_ = e.Put(1, []byte("a"))
	_ = e.Close()
	if err := e.Put(2, []byte("b")); err != ErrEngineClosed {
		t.Fatalf("got %v", err)
	}
}

func BenchmarkEngineConcurrentWrite(b *testing.B) {
	dir := b.TempDir()
	e, err := NewEngine(filepath.Join(dir, "b.bin"), filepath.Join(dir, "b.wal"), 1024)
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	payload := []byte(`{"name":"benchmark-record","val":12345}`)
	var mu sync.Mutex
	var next uint64
	b.SetParallelism(8)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			next++
			k := next
			mu.Unlock()
			if err := e.Put(k, payload); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
