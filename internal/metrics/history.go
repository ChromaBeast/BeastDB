package metrics

import (
	"sync"
	"time"
)

// Point is one sampled set of gauges. Counters are stored as rates already
// converted to per-second values by the sampler.
type Point struct {
	Time        int64   `json:"t"` // unix seconds
	LSN         uint64  `json:"lsn"`
	WALBytes    uint64  `json:"walBytes"`
	DirtyPages  int     `json:"dirtyPages"`
	CachedPages int     `json:"cachedPages"`
	ReqPerSec   float64 `json:"reqPerSec"`
	// ReplicaLag is the largest lag in LSNs over connected replicas (0 with none).
	ReplicaLag uint64 `json:"replicaLag"`
}

// History is a fixed-capacity in-memory ring of Points, oldest first. It is not
// persisted: it is cleared on restart. Safe for concurrent use.
type History struct {
	mu   sync.Mutex
	ring []Point
	next int
	n    int
}

// NewHistory returns a ring holding at most capacity points.
func NewHistory(capacity int) *History {
	if capacity <= 0 {
		capacity = 1
	}
	return &History{ring: make([]Point, capacity)}
}

// Add appends p, evicting the oldest point when full.
func (h *History) Add(p Point) {
	h.mu.Lock()
	h.ring[h.next] = p
	h.next = (h.next + 1) % len(h.ring)
	if h.n < len(h.ring) {
		h.n++
	}
	h.mu.Unlock()
}

// Window returns points newer than now-window, oldest first, thinned by an even
// stride so the result has at most maxPoints entries. The newest point is kept.
func (h *History) Window(now time.Time, window time.Duration, maxPoints int) []Point {
	cutoff := now.Add(-window).Unix()
	h.mu.Lock()
	all := make([]Point, 0, h.n)
	start := (h.next - h.n + len(h.ring)) % len(h.ring)
	for i := 0; i < h.n; i++ {
		p := h.ring[(start+i)%len(h.ring)]
		if p.Time >= cutoff {
			all = append(all, p)
		}
	}
	h.mu.Unlock()

	if maxPoints <= 0 || len(all) <= maxPoints {
		return all
	}
	stride := (len(all) + maxPoints - 1) / maxPoints
	out := make([]Point, 0, maxPoints)
	for i := len(all) - 1; i >= 0 && len(out) < maxPoints; i -= stride {
		out = append(out, all[i])
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// Total returns the number of requests recorded so far across all protocols
// and status codes. Used by samplers to derive a request rate.
func (r *RequestMetrics) Total() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n uint64
	for _, v := range r.count {
		n += v
	}
	return n
}
