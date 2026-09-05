package dsa

import (
	"errors"
)

var (
	// ErrHeapEmpty is returned when popping or peeking an empty heap.
	ErrHeapEmpty = errors.New("heap: is empty")
)

// Heap is an array-backed binary priority queue in pure Go without interface boxing.
type Heap[T any] struct {
	data []T
	less func(a, b T) bool
}

// NewHeap creates an empty heap with a custom comparator and initial capacity.
func NewHeap[T any](less func(a, b T) bool, initialCap int) *Heap[T] {
	if initialCap < 0 {
		initialCap = 0
	}
	return &Heap[T]{
		data: make([]T, 0, initialCap),
		less: less,
	}
}

// Len returns the number of elements currently in the heap.
func (h *Heap[T]) Len() int {
	return len(h.data)
}

// Push adds an item to the heap, maintaining the heap invariant via siftUp.
func (h *Heap[T]) Push(item T) {
	h.data = append(h.data, item)
	h.siftUp(len(h.data) - 1)
}

// Pop removes and returns the root element (min or max depending on comparator).
func (h *Heap[T]) Pop() (T, error) {
	n := len(h.data)
	if n == 0 {
		var zero T
		return zero, ErrHeapEmpty
	}
	root := h.data[0]
	n--
	h.data[0] = h.data[n]
	var zero T
	h.data[n] = zero // prevent memory leak
	h.data = h.data[:n]
	if n > 0 {
		h.siftDown(0)
	}
	return root, nil
}

// Peek returns the root element without removing it.
func (h *Heap[T]) Peek() (T, error) {
	if len(h.data) == 0 {
		var zero T
		return zero, ErrHeapEmpty
	}
	return h.data[0], nil
}

// Clear resets the heap size to 0 while keeping allocated capacity.
func (h *Heap[T]) Clear() {
	var zero T
	for i := range h.data {
		h.data[i] = zero
	}
	h.data = h.data[:0]
}

func (h *Heap[T]) siftUp(idx int) {
	for idx > 0 {
		parent := (idx - 1) >> 1
		if !h.less(h.data[idx], h.data[parent]) {
			break
		}
		h.data[idx], h.data[parent] = h.data[parent], h.data[idx]
		idx = parent
	}
}

func (h *Heap[T]) siftDown(idx int) {
	n := len(h.data)
	half := n >> 1
	for idx < half {
		left := (idx << 1) + 1
		right := left + 1
		candidate := left

		if right < n && h.less(h.data[right], h.data[left]) {
			candidate = right
		}
		if !h.less(h.data[candidate], h.data[idx]) {
			break
		}
		h.data[idx], h.data[candidate] = h.data[candidate], h.data[idx]
		idx = candidate
	}
}
