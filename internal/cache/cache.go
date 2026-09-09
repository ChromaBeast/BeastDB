package cache

import (
	"sync"
	"time"
)

// Cache is a thread-safe in-memory cache supporting O(1) LRU eviction and active/passive TTL.
type Cache[V any] struct {
	mu           sync.RWMutex
	maxCapacity  int
	items        map[string]*LRUNode[V]
	evictionList *LRUList[V]
	ttlQueue     *TTLHeap
	quit         chan struct{}
	wg           sync.WaitGroup
}

// NewCache initializes an LRU+TTL cache with an optional background expiration sweeper.
func NewCache[V any](maxCapacity int, sweepInterval time.Duration) *Cache[V] {
	if maxCapacity <= 0 {
		maxCapacity = 1000
	}

	c := &Cache[V]{
		maxCapacity:  maxCapacity,
		items:        make(map[string]*LRUNode[V]),
		evictionList: NewLRUList[V](),
		ttlQueue:     NewTTLHeap(64),
		quit:         make(chan struct{}),
	}

	if sweepInterval > 0 {
		c.wg.Add(1)
		go c.startSweeper(sweepInterval)
	}

	return c
}

// Set inserts or updates a key with an optional TTL.
// If the cache exceeds maxCapacity, the least recently used (LRU) element is evicted.
func (c *Cache[V]) Set(key string, val V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixMilli()
		c.ttlQueue.Push(key, expiresAt)
	}

	if node, ok := c.items[key]; ok {
		node.Value = val
		node.ExpiresAt = expiresAt
		c.evictionList.MoveToFront(node)
		return
	}

	if len(c.items) >= c.maxCapacity {
		if tail := c.evictionList.RemoveTail(); tail != nil {
			delete(c.items, tail.Key)
		}
	}

	node := &LRUNode[V]{
		Key:       key,
		Value:     val,
		ExpiresAt: expiresAt,
	}
	c.evictionList.PushFront(node)
	c.items[key] = node
}

// Get fetches a key, promotes it to MRU, and passively evicts it if expired.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	if node.ExpiresAt > 0 && time.Now().UnixMilli() >= node.ExpiresAt {
		c.evictionList.Remove(node)
		delete(c.items, key)
		var zero V
		return zero, false
	}

	c.evictionList.MoveToFront(node)
	return node.Value, true
}

// Delete removes a key and its node from the LRU list.
func (c *Cache[V]) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return false
	}

	c.evictionList.Remove(node)
	delete(c.items, key)
	return true
}

// Len returns the number of active items currently stored.
func (c *Cache[V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// EvictExpired proactively purges all expired keys using the Min-Heap.
func (c *Cache[V]) EvictExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixMilli()
	evicted := 0

	for {
		entry, ok := c.ttlQueue.Peek()
		if !ok || entry.ExpiresAt > now {
			break
		}
		c.ttlQueue.Pop()

		if node, exists := c.items[entry.Key]; exists && node.ExpiresAt <= now {
			c.evictionList.Remove(node)
			delete(c.items, entry.Key)
			evicted++
		}
	}

	return evicted
}

// Close terminates the background expiration sweeper cleanly.
func (c *Cache[V]) Close() {
	close(c.quit)
	c.wg.Wait()
}

func (c *Cache[V]) startSweeper(interval time.Duration) {
	defer c.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.EvictExpired()
		case <-c.quit:
			return
		}
	}
}
