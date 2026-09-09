package cache

import (
	"sync/atomic"

	"github.com/ChromaBeast/beastdb/internal/dsa"
)

// Metrics provides high-performance, lock-free operational counters.
// CacheLinePad ensures hot atomic variables reside on separate L1 cache lines,
// eliminating CPU cache-coherence invalidation (false sharing).
type Metrics struct {
	totalQueries uint64
	_            dsa.CacheLinePad
	totalGets    uint64
	_            dsa.CacheLinePad
	totalSets    uint64
	_            dsa.CacheLinePad
	totalDels    uint64
	_            dsa.CacheLinePad
	totalHits    uint64
	_            dsa.CacheLinePad
	totalMisses  uint64
}

// IncrQueries increments the total query count atomically.
func (m *Metrics) IncrQueries() {
	atomic.AddUint64(&m.totalQueries, 1)
}

// IncrGets increments the total GET operations atomically.
func (m *Metrics) IncrGets() {
	atomic.AddUint64(&m.totalGets, 1)
}

// IncrSets increments the total SET operations atomically.
func (m *Metrics) IncrSets() {
	atomic.AddUint64(&m.totalSets, 1)
}

// IncrDels increments the total DEL operations atomically.
func (m *Metrics) IncrDels() {
	atomic.AddUint64(&m.totalDels, 1)
}

// IncrHits increments the cache hit count atomically.
func (m *Metrics) IncrHits() {
	atomic.AddUint64(&m.totalHits, 1)
}

// IncrMisses increments the cache miss count atomically.
func (m *Metrics) IncrMisses() {
	atomic.AddUint64(&m.totalMisses, 1)
}

// MetricsSnapshot holds a point-in-time read of all telemetry counters.
type MetricsSnapshot struct {
	TotalQueries uint64
	TotalGets    uint64
	TotalSets    uint64
	TotalDels    uint64
	TotalHits    uint64
	TotalMisses  uint64
}

// Snapshot returns an atomic point-in-time copy of database metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalQueries: atomic.LoadUint64(&m.totalQueries),
		TotalGets:    atomic.LoadUint64(&m.totalGets),
		TotalSets:    atomic.LoadUint64(&m.totalSets),
		TotalDels:    atomic.LoadUint64(&m.totalDels),
		TotalHits:    atomic.LoadUint64(&m.totalHits),
		TotalMisses:  atomic.LoadUint64(&m.totalMisses),
	}
}
