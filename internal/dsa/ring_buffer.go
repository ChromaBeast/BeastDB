package dsa

import (
	"errors"
	"math/bits"
)

var (
	// ErrRingBufferFull is returned when pushing to a full ring buffer.
	ErrRingBufferFull = errors.New("ring buffer: buffer is full")
	// ErrRingBufferEmpty is returned when popping from an empty ring buffer.
	ErrRingBufferEmpty = errors.New("ring buffer: buffer is empty")
)

// RingBuffer is a high-performance circular queue with power-of-two capacity.
// It uses bitwise masking (& (cap - 1)) instead of division for O(1) operations.
type RingBuffer[T any] struct {
	data  []T
	mask  int
	head  int
	tail  int
	count int
}

// NewRingBuffer creates a ring buffer with capacity rounded up to the nearest power of two.
func NewRingBuffer[T any](minCap int) *RingBuffer[T] {
	if minCap < 2 {
		minCap = 2
	}
	capacity := nextPowerOfTwo(minCap)
	return &RingBuffer[T]{
		data: make([]T, capacity),
		mask: capacity - 1,
	}
}

// Push adds an item to the tail of the buffer.
func (r *RingBuffer[T]) Push(item T) error {
	if r.count == len(r.data) {
		return ErrRingBufferFull
	}
	r.data[r.tail] = item
	r.tail = (r.tail + 1) & r.mask
	r.count++
	return nil
}

// Pop removes and returns an item from the head of the buffer.
func (r *RingBuffer[T]) Pop() (T, error) {
	if r.count == 0 {
		var zero T
		return zero, ErrRingBufferEmpty
	}
	item := r.data[r.head]
	var zero T
	r.data[r.head] = zero // avoid memory leaks
	r.head = (r.head + 1) & r.mask
	r.count--
	return item, nil
}

// Peek returns the item at head without removing it.
func (r *RingBuffer[T]) Peek() (T, error) {
	if r.count == 0 {
		var zero T
		return zero, ErrRingBufferEmpty
	}
	return r.data[r.head], nil
}

// Len returns the current count of stored items.
func (r *RingBuffer[T]) Len() int {
	return r.count
}

// Cap returns the total capacity of the ring buffer.
func (r *RingBuffer[T]) Cap() int {
	return len(r.data)
}

// IsFull returns true if no more items can be pushed.
func (r *RingBuffer[T]) IsFull() bool {
	return r.count == len(r.data)
}

// IsEmpty returns true if there are no items in the buffer.
func (r *RingBuffer[T]) IsEmpty() bool {
	return r.count == 0
}

// Clear resets head, tail, and count while clearing references.
func (r *RingBuffer[T]) Clear() {
	var zero T
	for i := range r.data {
		r.data[i] = zero
	}
	r.head = 0
	r.tail = 0
	r.count = 0
}

func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	if n&(n-1) == 0 {
		return n
	}
	return 1 << (bits.Len(uint(n)))
}
