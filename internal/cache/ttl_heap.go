package cache

import (
	"github.com/ChromaBeast/beastdb/internal/dsa"
)

// TTLEntry associates a database key with its expiration timestamp (Unix milliseconds).
type TTLEntry struct {
	Key       string
	ExpiresAt int64
}

// TTLHeap wraps our pure Go MinHeap to maintain expiration order.
// The earliest expiring key is always at the root in O(1) peek time.
type TTLHeap struct {
	heap *dsa.Heap[TTLEntry]
}

// NewTTLHeap initializes a priority queue ordered by earliest expiration timestamp.
func NewTTLHeap(initialCap int) *TTLHeap {
	return &TTLHeap{
		heap: dsa.NewHeap[TTLEntry](func(a, b TTLEntry) bool {
			return a.ExpiresAt < b.ExpiresAt
		}, initialCap),
	}
}

// Push adds a key-expiration pair to the priority queue.
func (h *TTLHeap) Push(key string, expiresAt int64) {
	h.heap.Push(TTLEntry{
		Key:       key,
		ExpiresAt: expiresAt,
	})
}

// Peek returns the earliest expiring entry without removing it.
func (h *TTLHeap) Peek() (TTLEntry, bool) {
	entry, err := h.heap.Peek()
	if err != nil {
		return TTLEntry{}, false
	}
	return entry, true
}

// Pop removes and returns the earliest expiring entry.
func (h *TTLHeap) Pop() (TTLEntry, bool) {
	entry, err := h.heap.Pop()
	if err != nil {
		return TTLEntry{}, false
	}
	return entry, true
}

// Len returns the number of tracked TTL entries.
func (h *TTLHeap) Len() int {
	return h.heap.Len()
}

// Clear resets the priority queue.
func (h *TTLHeap) Clear() {
	h.heap.Clear()
}
