package dsa

import (
	"fmt"
	"testing"
)

func TestHashTableBasic(t *testing.T) {
	ht := NewHashTable[string](8)

	ht.Set("user:101", "Alice")
	ht.Set("user:102", "Bob")
	ht.Set("user:103", "Charlie")

	if ht.Len() != 3 {
		t.Fatalf("expected len=3, got %d", ht.Len())
	}

	val, ok := ht.Get("user:101")
	if !ok || val != "Alice" {
		t.Fatalf("expected Alice, got %q (ok=%v)", val, ok)
	}

	// Update existing key
	ht.Set("user:101", "Alice_Updated")
	val, ok = ht.Get("user:101")
	if !ok || val != "Alice_Updated" {
		t.Fatalf("expected updated value, got %q", val)
	}
	if ht.Len() != 3 {
		t.Fatalf("len should remain 3 after update, got %d", ht.Len())
	}

	// Delete key
	if !ht.Delete("user:102") {
		t.Fatalf("expected successful deletion of user:102")
	}
	if ht.Delete("user:102") {
		t.Fatalf("expected second delete to return false")
	}

	_, ok = ht.Get("user:102")
	if ok {
		t.Fatalf("expected deleted key to return ok=false")
	}
}

func TestHashTableTombstonePreservation(t *testing.T) {
	ht := NewHashTable[int](16)

	// Insert items that share hash slots or form a probe sequence
	for i := 0; i < 8; i++ {
		ht.Set(fmt.Sprintf("key_%d", i), i*10)
	}

	// Delete every alternate key (leaves tombstones)
	for i := 0; i < 8; i += 2 {
		ht.Delete(fmt.Sprintf("key_%d", i))
	}

	// Verify remaining keys can still be retrieved across tombstones
	for i := 1; i < 8; i += 2 {
		val, ok := ht.Get(fmt.Sprintf("key_%d", i))
		if !ok || val != i*10 {
			t.Fatalf("failed to retrieve key_%d across tombstones: got %d (ok=%v)", i, val, ok)
		}
	}
}

func TestHashTableGrowth(t *testing.T) {
	ht := NewHashTable[int](8)
	initialCap := ht.Cap()

	// Insert 100 keys to force multiple resizes
	for i := 0; i < 100; i++ {
		ht.Set(fmt.Sprintf("k_%d", i), i)
	}

	if ht.Cap() <= initialCap {
		t.Fatalf("expected capacity to grow beyond %d, got %d", initialCap, ht.Cap())
	}

	// Verify all 100 keys are intact after resizing
	for i := 0; i < 100; i++ {
		val, ok := ht.Get(fmt.Sprintf("k_%d", i))
		if !ok || val != i {
			t.Fatalf("key k_%d corrupted after resize: got %d, expected %d", i, val, i)
		}
	}
}

func BenchmarkHashTableGet(b *testing.B) {
	ht := NewHashTable[string](1024)
	for i := 0; i < 500; i++ {
		ht.Set(fmt.Sprintf("user:id:%d", i), "payload")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = ht.Get("user:id:250")
	}
}
