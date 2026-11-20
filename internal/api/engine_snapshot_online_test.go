package api

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// snapshotWhileWriting takes a snapshot of eng with the copy held open by a
// hook, proves reads and writes proceed and checkpoints wait during the copy,
// and returns the snapshot path with the number of keys the snapshot must hold.
func snapshotWhileWriting(t *testing.T, eng *Engine, before, during int) string {
	t.Helper()
	inCopy, release := make(chan struct{}), make(chan struct{})
	eng.snapshotHook = func() { close(inCopy); <-release }

	type res struct {
		path string
		err  error
	}
	done := make(chan res, 1)
	go func() {
		p, err := eng.CreateSnapshot(filepath.Join(t.TempDir(), "snap"))
		done <- res{p, err}
	}()
	<-inCopy // checkpoint taken, copy has not started

	// Writes and reads must not wait for the copy.
	writes := make(chan error, 1)
	go func() {
		for i := before + 1; i <= before+during; i++ {
			if err := eng.Put(uint64(i), value(i)); err != nil {
				writes <- err
				return
			}
		}
		writes <- nil
	}()
	select {
	case err := <-writes:
		if err != nil {
			t.Fatalf("write during snapshot: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writes stalled while the snapshot copy was running")
	}
	if _, ok, err := eng.Get(1); err != nil || !ok {
		t.Fatalf("read during snapshot: ok=%v err=%v", ok, err)
	}

	// A checkpoint would rewrite the file being copied: it must wait.
	var ckptDone atomic.Bool
	ckptErr := make(chan error, 1)
	go func() { err := eng.Checkpoint(); ckptDone.Store(true); ckptErr <- err }()
	time.Sleep(150 * time.Millisecond)
	if ckptDone.Load() {
		t.Fatal("a checkpoint ran while the snapshot was copying the data file")
	}

	close(release)
	r := <-done
	if r.err != nil {
		t.Fatalf("snapshot: %v", r.err)
	}
	if err := <-ckptErr; err != nil {
		t.Fatalf("checkpoint after snapshot: %v", err)
	}
	return r.path
}

func assertKeys(t *testing.T, eng *Engine, present, absentFrom, absentTo int) {
	t.Helper()
	checkEngine(t, eng, present)
	for i := absentFrom; i <= absentTo; i++ {
		if _, ok, err := eng.Get(uint64(i)); err != nil || ok {
			t.Fatalf("key %d is in the snapshot but was written after it: ok=%v err=%v", i, ok, err)
		}
	}
}

func TestSnapshotDoesNotBlockWritesAndIsConsistent(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewEngine(filepath.Join(dir, "a.bin"), filepath.Join(dir, "a.wal"), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	fillEngine(t, eng, 300)
	snap := snapshotWhileWriting(t, eng, 300, 200)

	restored, err := NewEngine(snap, filepath.Join(t.TempDir(), "r.wal"), 64)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer restored.Close()
	assertKeys(t, restored, 300, 301, 500)
	checkEngine(t, eng, 500) // the live engine kept every write
}

func TestEncryptedSnapshotDoesNotBlockWritesAndIsConsistent(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 64)
	defer eng.Close()
	fillEngine(t, eng, 300)
	snap := snapshotWhileWriting(t, eng, 300, 200)

	if fileHasMarker(t, snap) {
		t.Fatal("encrypted snapshot holds plaintext")
	}
	kr, err := os.ReadFile(eng.KeyringPath()) // the backup package ships this in the bundle
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snap+".keyring", kr, 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := NewEngineWithOptions(snap, filepath.Join(t.TempDir(), "r.wal"), 64, Options{KeyFile: env.key})
	if err != nil {
		t.Fatalf("open encrypted snapshot: %v", err)
	}
	defer restored.Close()
	assertKeys(t, restored, 300, 301, 500)
	checkEngine(t, eng, 500)
}

// With writers running flat out (and a small pool, so pages spill) every
// snapshot must open and hold a gap-free prefix of the sequential key stream.
func TestSnapshotUnderContinuousWrites(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewEngine(filepath.Join(dir, "a.bin"), filepath.Join(dir, "a.wal"), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	var next atomic.Uint64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			k := next.Add(1)
			if err := eng.Put(k, value(int(k))); err != nil {
				return
			}
		}
	}()
	for next.Load() < 200 {
		time.Sleep(time.Millisecond)
	}
	var snaps []string
	for i := 0; i < 4; i++ {
		p, err := eng.CreateSnapshot(filepath.Join(dir, "snaps"))
		if err != nil {
			t.Fatalf("snapshot %d: %v", i, err)
		}
		snaps = append(snaps, p)
		time.Sleep(20 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	for _, p := range snaps {
		r, err := NewEngine(p, filepath.Join(t.TempDir(), "r.wal"), 64)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		n := uint64(0)
		for {
			v, ok, err := r.Get(n + 1)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if !bytes.Equal(v, value(int(n+1))) {
				t.Fatalf("snapshot %s: key %d has wrong value", p, n+1)
			}
			n++
		}
		if n < 200 {
			t.Fatalf("snapshot %s holds only %d keys", p, n)
		}
		// Nothing past the prefix: the snapshot is a point in time, not a smear.
		for k := n + 2; k <= next.Load(); k += 7 {
			if _, ok, _ := r.Get(k); ok {
				t.Fatalf("snapshot %s has key %d after a gap at %d", p, k, n+1)
			}
		}
		r.Close()
	}
}
