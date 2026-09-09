package cache

import (
	"fmt"
	"sync"
	"testing"
)

func TestShardedMapBasic(t *testing.T) {
	sm := NewShardedMap[string](16)

	sm.Set("key1", "val1")
	sm.Set("key2", "val2")

	val, ok := sm.Get("key1")
	if !ok || val != "val1" {
		t.Fatalf("expected val1, got %q (ok=%v)", val, ok)
	}

	if sm.Len() != 2 {
		t.Fatalf("expected len 2, got %d", sm.Len())
	}

	if !sm.Delete("key1") {
		t.Fatalf("expected key1 to be deleted")
	}

	_, ok = sm.Get("key1")
	if ok {
		t.Fatalf("expected deleted key1 to return ok=false")
	}

	if sm.Len() != 1 {
		t.Fatalf("expected len 1 after deletion, got %d", sm.Len())
	}
}

func TestShardedMapConcurrent(t *testing.T) {
	sm := NewShardedMap[int](32)
	numGoroutines := 100
	opsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("goroutine_%d_key_%d", gID, i)
				sm.Set(key, i)

				val, ok := sm.Get(key)
				if !ok || val != i {
					t.Errorf("concurrent get failed for %s: got %d (ok=%v)", key, val, ok)
				}
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := numGoroutines * opsPerGoroutine
	if sm.Len() != expectedTotal {
		t.Fatalf("expected total len %d, got %d", expectedTotal, sm.Len())
	}

	snap := sm.Metrics().Snapshot()
	if snap.TotalSets != uint64(expectedTotal) {
		t.Fatalf("expected %d sets, got %d", expectedTotal, snap.TotalSets)
	}
	if snap.TotalGets != uint64(expectedTotal) {
		t.Fatalf("expected %d gets, got %d", expectedTotal, snap.TotalGets)
	}
	if snap.TotalHits != uint64(expectedTotal) {
		t.Fatalf("expected %d hits, got %d", expectedTotal, snap.TotalHits)
	}
}

func BenchmarkShardedMapConcurrent(b *testing.B) {
	sm := NewShardedMap[string](64)
	for i := 0; i < 1000; i++ {
		sm.Set(fmt.Sprintf("bench_key_%d", i), "payload")
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i%1000)
			_, _ = sm.Get(key)
			i++
		}
	})
}
