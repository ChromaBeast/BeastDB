package dsa

import (
	"errors"
)

var (
	// ErrIndexOutOfBounds is returned when an index is outside [0, len).
	ErrIndexOutOfBounds = errors.New("vector: index out of bounds")
	// ErrEmptyVector is returned when popping from an empty vector.
	ErrEmptyVector = errors.New("vector: is empty")
)

// Vector is a contiguous, generic dynamic array with amortized O(1) appends.
// It uses a 1.5x growth factor to minimize fragmentation and memory overhead.
type Vector[T any] struct {
	data []T
	len  int
}

// NewVector initializes a Vector with a pre-allocated capacity.
func NewVector[T any](initialCap int) *Vector[T] {
	if initialCap < 0 {
		initialCap = 0
	}
	return &Vector[T]{
		data: make([]T, initialCap),
		len:  0,
	}
}

// Len returns the number of active elements in the vector.
func (v *Vector[T]) Len() int {
	return v.len
}

// Cap returns the total allocated capacity of the vector.
func (v *Vector[T]) Cap() int {
	return len(v.data)
}

// Push appends an element, growing the underlying slice by 1.5x if needed.
func (v *Vector[T]) Push(item T) {
	if v.len == len(v.data) {
		v.grow()
	}
	v.data[v.len] = item
	v.len++
}

// Pop removes and returns the last element.
func (v *Vector[T]) Pop() (T, error) {
	if v.len == 0 {
		var zero T
		return zero, ErrEmptyVector
	}
	v.len--
	item := v.data[v.len]
	var zero T
	v.data[v.len] = zero // prevent memory leak for pointer types
	return item, nil
}

// Get returns the element at the specified index without removing it.
func (v *Vector[T]) Get(idx int) (T, error) {
	if idx < 0 || idx >= v.len {
		var zero T
		return zero, ErrIndexOutOfBounds
	}
	return v.data[idx], nil
}

// Set updates the element at the specified index.
func (v *Vector[T]) Set(idx int, item T) error {
	if idx < 0 || idx >= v.len {
		return ErrIndexOutOfBounds
	}
	v.data[idx] = item
	return nil
}

// Clear resets the vector length to 0 while keeping capacity allocated.
func (v *Vector[T]) Clear() {
	var zero T
	for i := 0; i < v.len; i++ {
		v.data[i] = zero
	}
	v.len = 0
}

// ShrinkToFit reallocates memory to match the exact length if cap > len.
func (v *Vector[T]) ShrinkToFit() {
	if v.len == len(v.data) {
		return
	}
	newData := make([]T, v.len)
	copy(newData, v.data[:v.len])
	v.data = newData
}

func (v *Vector[T]) grow() {
	currentCap := len(v.data)
	var newCap int
	if currentCap == 0 {
		newCap = 4
	} else {
		newCap = currentCap + (currentCap >> 1) // 1.5x growth
		if newCap <= currentCap {
			newCap = currentCap + 1
		}
	}
	newData := make([]T, newCap)
	copy(newData, v.data[:v.len])
	v.data = newData
}
