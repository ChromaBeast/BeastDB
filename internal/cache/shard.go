package cache

import (
	"math/bits"
	"sync"

	"github.com/ChromaBeast/beastdb/internal/dsa"
)

// Shard holds an isolated mutex lock and an independent open-addressing table.
type Shard[V any] struct {
	mu    sync.RWMutex
	table *dsa.HashTable[V]
}

// ShardedMap divides database keys across N independent shards to eliminate lock contention.
type ShardedMap[V any] struct {
	shards  []*Shard[V]
	mask    int
	metrics *Metrics
}

// NewShardedMap initializes a partitioned map with power-of-two shards (default 64).
func NewShardedMap[V any](shardCount int) *ShardedMap[V] {
	if shardCount < 1 {
		shardCount = 64
	}
	capacity := nextPowerOfTwo(shardCount)
	shards := make([]*Shard[V], capacity)
	for i := 0; i < capacity; i++ {
		shards[i] = &Shard[V]{
			table: dsa.NewHashTable[V](16),
		}
	}

	return &ShardedMap[V]{
		shards:  shards,
		mask:    capacity - 1,
		metrics: &Metrics{},
	}
}

// Metrics returns the database operational metrics.
func (s *ShardedMap[V]) Metrics() *Metrics {
	return s.metrics
}

// Get performs a concurrent shared-read lookup on the target shard.
func (s *ShardedMap[V]) Get(key string) (V, bool) {
	s.metrics.IncrQueries()
	s.metrics.IncrGets()

	idx := int(dsa.HashString(key)) & s.mask
	shard := s.shards[idx]

	shard.mu.RLock()
	val, ok := shard.table.Get(key)
	shard.mu.RUnlock()

	if ok {
		s.metrics.IncrHits()
	} else {
		s.metrics.IncrMisses()
	}
	return val, ok
}

// Set performs an isolated write on the target shard without blocking other shards.
func (s *ShardedMap[V]) Set(key string, val V) {
	s.metrics.IncrQueries()
	s.metrics.IncrSets()

	idx := int(dsa.HashString(key)) & s.mask
	shard := s.shards[idx]

	shard.mu.Lock()
	shard.table.Set(key, val)
	shard.mu.Unlock()
}

// Delete removes a key from its designated shard.
func (s *ShardedMap[V]) Delete(key string) bool {
	s.metrics.IncrQueries()
	s.metrics.IncrDels()

	idx := int(dsa.HashString(key)) & s.mask
	shard := s.shards[idx]

	shard.mu.Lock()
	deleted := shard.table.Delete(key)
	shard.mu.Unlock()

	return deleted
}

// Len calculates the total count of active elements across all shards.
func (s *ShardedMap[V]) Len() int {
	total := 0
	for _, shard := range s.shards {
		shard.mu.RLock()
		total += shard.table.Len()
		shard.mu.RUnlock()
	}
	return total
}

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	if n&(n-1) == 0 {
		return n
	}
	return 1 << bits.Len(uint(n))
}
