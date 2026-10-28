package api

import (
	"bytes"
	"errors"
	"math/rand"
	"path/filepath"
	"testing"
)

func openLarge(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := NewEngine(filepath.Join(dir, "large.bin"), filepath.Join(dir, "large.wal"), 64)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	return e
}

func pattern(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31) ^ seed
	}
	return b
}

func mustGet(t *testing.T, e *Engine, key uint64, want []byte) {
	t.Helper()
	got, found, err := e.Get(key)
	if err != nil || !found {
		t.Fatalf("get %d: found=%v err=%v", key, found, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("get %d: %d bytes back, want %d", key, len(got), len(want))
	}
}

func TestLargeValueRoundTripAtBoundaries(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	sizes := []int{0, 1, MaxRecordSize, MaxRecordSize + 1, chunkPayload * 3, chunkPayload*3 + 1, 1 << 20, MaxValueSize}
	for i, n := range sizes {
		if err := e.Put(uint64(i+1), pattern(n, byte(i))); err != nil {
			t.Fatalf("put %d bytes: %v", n, err)
		}
	}
	for i, n := range sizes {
		mustGet(t, e, uint64(i+1), pattern(n, byte(i)))
	}
	if err := e.Put(99, make([]byte, MaxValueSize+1)); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("expected ErrRecordTooLarge, got %v", err)
	}
}

func TestLargeValueOverwriteAndDeleteReuseSpace(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	val := pattern(512<<10, 7)
	for i := 0; i < 5; i++ {
		if err := e.Put(1, val); err != nil {
			t.Fatal(err)
		}
	}
	afterWarmup := e.disk.NumPages()
	for i := 0; i < 40; i++ {
		_ = e.Put(1, pattern(512<<10, byte(i)))
	}
	if grown := e.disk.NumPages() - afterWarmup; grown > afterWarmup/2 {
		t.Fatalf("overwrites leaked pages: %d pages after warm-up, %d more after 40 overwrites", afterWarmup, grown)
	}
	if err := e.Put(1, []byte("small again")); err != nil {
		t.Fatal(err)
	}
	mustGet(t, e, 1, []byte("small again"))
	if err := e.Delete(1); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := e.Get(1); found {
		t.Fatal("deleted large value still readable")
	}
}

func TestLargeValueSurvivesCrashAndCleanRestart(t *testing.T) {
	dir := t.TempDir()
	e := openLarge(t, dir)
	_ = e.Put(1, pattern(300<<10, 1))
	_ = e.Put(2, pattern(10, 2))
	_ = e.BatchWrite([]BatchOperation{{Type: BatchOpPut, Key: 3, Value: pattern(90<<10, 3)}, {Type: BatchOpDelete, Key: 2}})
	// Abrupt stop: WAL durable, pages never flushed.
	_ = e.wal.Sync()
	_ = e.wal.Close()
	_ = e.disk.Close()

	e = openLarge(t, dir)
	mustGet(t, e, 1, pattern(300<<10, 1))
	mustGet(t, e, 3, pattern(90<<10, 3))
	if _, found, _ := e.Get(2); found {
		t.Fatal("batch delete lost in recovery")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openLarge(t, dir)
	defer e.Close()
	mustGet(t, e, 1, pattern(300<<10, 1))
}

func TestLargeValueConditionsBatchesAndScans(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	v1, v2 := pattern(50<<10, 1), pattern(60<<10, 2)
	if err := e.PutIf(1, v1, Condition{Kind: CondAbsent}); err != nil {
		t.Fatal(err)
	}
	if err := e.PutIf(1, v2, Condition{Kind: CondValueEquals, Value: v2}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("stale compare-and-swap must fail, got %v", err)
	}
	if err := e.PutIf(1, v2, Condition{Kind: CondValueEquals, Value: v1}); err != nil {
		t.Fatalf("compare-and-swap on a large value: %v", err)
	}
	big := []BatchOperation{{Type: BatchOpPut, Key: 2, Value: make([]byte, MaxValueSize)}, {Type: BatchOpPut, Key: 3, Value: make([]byte, MaxValueSize)},
		{Type: BatchOpPut, Key: 4, Value: make([]byte, MaxValueSize)}, {Type: BatchOpPut, Key: 5, Value: make([]byte, MaxValueSize)},
		{Type: BatchOpPut, Key: 6, Value: []byte("x")}}
	if err := e.BatchWrite(big); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("expected ErrBatchTooLarge, got %v", err)
	}
	cur, err := e.Scan(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, rid, ok, err := cur.Next()
	if err != nil || !ok {
		t.Fatalf("scan found nothing: %v", err)
	}
	got, err := e.ReadTuple(rid)
	_ = cur.Close()
	if err != nil || !bytes.Equal(got, v2) {
		t.Fatalf("scan must return the whole large value: %d bytes, %v", len(got), err)
	}
}

func TestCorruptChunkIsDetected(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	_ = e.Put(1, pattern(20<<10, 9))
	rid, _ := e.tree.Find(1)
	m, _ := e.readManifest(rid)
	page, _ := e.bpm.FetchPage(m.first.PageID)
	tuple, _ := page.GetTuple(m.first.SlotID)
	tuple[chunkHeader+5] ^= 0xFF
	_ = e.bpm.UnpinPage(m.first.PageID, true)
	if _, _, err := e.Get(1); !errors.Is(err, ErrCorruptValue) {
		t.Fatalf("expected ErrCorruptValue, got %v", err)
	}
}

func TestMixedSizeOracleAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	e := openLarge(t, dir)
	oracle := map[uint64][]byte{}
	rng := rand.New(rand.NewSource(42))
	for round := 0; round < 4; round++ {
		for op := 0; op < 80; op++ {
			key := uint64(rng.Intn(30) + 1)
			if rng.Intn(4) == 0 {
				_ = e.Delete(key)
				delete(oracle, key)
				continue
			}
			sizes := []int{rng.Intn(200), rng.Intn(MaxRecordSize), MaxRecordSize + rng.Intn(20000), rng.Intn(200 << 10)}
			val := pattern(sizes[rng.Intn(len(sizes))], byte(op))
			if err := e.Put(key, val); err != nil {
				t.Fatalf("put: %v", err)
			}
			oracle[key] = val
		}
		_ = e.Close()
		e = openLarge(t, dir)
		for k, v := range oracle {
			mustGet(t, e, k, v)
		}
	}
	_ = e.Close()
}
