package cache

import (
	"fmt"
	"testing"
	"time"
)

func TestCacheLRUEviction(t *testing.T) {
	cache := NewCache[string](3, 0)

	cache.Set("k1", "v1", 0)
	cache.Set("k2", "v2", 0)
	cache.Set("k3", "v3", 0)

	// Access k1 to promote it to Most Recently Used (MRU)
	val, ok := cache.Get("k1")
	if !ok || val != "v1" {
		t.Fatalf("expected k1=v1, got %q (ok=%v)", val, ok)
	}

	// Insert k4 -> should evict k2 (least recently used, since k1 was accessed!)
	cache.Set("k4", "v4", 0)

	if cache.Len() != 3 {
		t.Fatalf("expected len 3, got %d", cache.Len())
	}

	// k2 must be evicted
	if _, ok := cache.Get("k2"); ok {
		t.Fatalf("expected k2 to be evicted by LRU")
	}

	// k1, k3, k4 must still exist
	for _, key := range []string{"k1", "k3", "k4"} {
		if _, ok := cache.Get(key); !ok {
			t.Fatalf("expected %s to still exist in cache", key)
		}
	}
}

func TestCachePassiveTTLExpiration(t *testing.T) {
	cache := NewCache[string](10, 0)

	// Set with 20ms TTL
	cache.Set("short_lived", "data", 20*time.Millisecond)

	// Immediate lookup succeeds
	if val, ok := cache.Get("short_lived"); !ok || val != "data" {
		t.Fatalf("expected key to exist immediately, got ok=%v", ok)
	}

	// Wait for expiration
	time.Sleep(30 * time.Millisecond)

	// Passive on-read check should evict and return false
	if _, ok := cache.Get("short_lived"); ok {
		t.Fatalf("expected short_lived to expire passively on read")
	}

	if cache.Len() != 0 {
		t.Fatalf("expected cache len 0 after passive eviction, got %d", cache.Len())
	}
}

func TestCacheActiveTTLSweep(t *testing.T) {
	// Cache with fast active sweeper (10ms ticker)
	cache := NewCache[string](10, 10*time.Millisecond)
	defer cache.Close()

	cache.Set("active_expire", "val", 25*time.Millisecond)

	// Wait for background sweeper to purge it
	time.Sleep(50 * time.Millisecond)

	// Active background sweep must have cleaned it without any Get() call!
	if cache.Len() != 0 {
		t.Fatalf("expected active sweeper to reduce len to 0, got %d", cache.Len())
	}
}

func BenchmarkCacheSetGet(b *testing.B) {
	cache := NewCache[string](1024, 0)
	for i := 0; i < 500; i++ {
		cache.Set(fmt.Sprintf("key_%d", i), "data", 0)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%500)
		_, _ = cache.Get(key)
	}
}
