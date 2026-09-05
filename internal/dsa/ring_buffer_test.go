package dsa

import (
	"testing"
)

func TestRingBufferBasic(t *testing.T) {
	rb := NewRingBuffer[int](4)
	if rb.Cap() != 4 {
		t.Fatalf("expected cap=4, got %d", rb.Cap())
	}
	if !rb.IsEmpty() {
		t.Fatalf("expected buffer to be empty")
	}

	for i := 1; i <= 4; i++ {
		if err := rb.Push(i); err != nil {
			t.Fatalf("failed to push %d: %v", i, err)
		}
	}

	if !rb.IsFull() {
		t.Fatalf("expected buffer to be full")
	}

	// Overfill should return ErrRingBufferFull
	if err := rb.Push(5); err != ErrRingBufferFull {
		t.Fatalf("expected ErrRingBufferFull, got %v", err)
	}

	// Peek
	peekVal, err := rb.Peek()
	if err != nil || peekVal != 1 {
		t.Fatalf("expected peek 1, got %d (err: %v)", peekVal, err)
	}

	// Pop all
	for i := 1; i <= 4; i++ {
		val, err := rb.Pop()
		if err != nil || val != i {
			t.Fatalf("expected pop %d, got %d (err: %v)", i, val, err)
		}
	}

	if !rb.IsEmpty() {
		t.Fatalf("expected buffer to be empty after popping all")
	}

	// Pop from empty
	_, err = rb.Pop()
	if err != ErrRingBufferEmpty {
		t.Fatalf("expected ErrRingBufferEmpty, got %v", err)
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer[int](4)

	// Push 3, Pop 2, Push 3 (forces wrap-around across circular array)
	rb.Push(10)
	rb.Push(20)
	rb.Push(30)

	v1, _ := rb.Pop()
	v2, _ := rb.Pop()
	if v1 != 10 || v2 != 20 {
		t.Fatalf("unexpected pop values: %d, %d", v1, v2)
	}

	rb.Push(40)
	rb.Push(50)
	rb.Push(60)

	expected := []int{30, 40, 50, 60}
	for _, exp := range expected {
		val, err := rb.Pop()
		if err != nil || val != exp {
			t.Fatalf("expected %d, got %d (err: %v)", exp, val, err)
		}
	}
}

func BenchmarkRingBufferPushPop(b *testing.B) {
	rb := NewRingBuffer[int](1024)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = rb.Push(i)
		_, _ = rb.Pop()
	}
}
