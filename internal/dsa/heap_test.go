package dsa

import (
	"math/rand"
	"testing"
)

func TestMinHeap(t *testing.T) {
	minHeap := NewHeap[int](func(a, b int) bool { return a < b }, 16)

	vals := []int{42, 10, 5, 100, 1, 23, 7}
	for _, v := range vals {
		minHeap.Push(v)
	}

	if minHeap.Len() != len(vals) {
		t.Fatalf("expected len=%d, got %d", len(vals), minHeap.Len())
	}

	peek, err := minHeap.Peek()
	if err != nil || peek != 1 {
		t.Fatalf("expected min element 1, got %d (err: %v)", peek, err)
	}

	var sorted []int
	for minHeap.Len() > 0 {
		v, err := minHeap.Pop()
		if err != nil {
			t.Fatalf("pop error: %v", err)
		}
		sorted = append(sorted, v)
	}

	expected := []int{1, 5, 7, 10, 23, 42, 100}
	for i, exp := range expected {
		if sorted[i] != exp {
			t.Fatalf("at index %d: expected %d, got %d", i, exp, sorted[i])
		}
	}

	_, err = minHeap.Pop()
	if err != ErrHeapEmpty {
		t.Fatalf("expected ErrHeapEmpty, got %v", err)
	}
}

func TestMaxHeap(t *testing.T) {
	maxHeap := NewHeap[int](func(a, b int) bool { return a > b }, 8)

	for _, v := range []int{15, 3, 22, 8, 50} {
		maxHeap.Push(v)
	}

	v, _ := maxHeap.Pop()
	if v != 50 {
		t.Fatalf("expected max element 50, got %d", v)
	}
	v, _ = maxHeap.Pop()
	if v != 22 {
		t.Fatalf("expected next element 22, got %d", v)
	}
}

func BenchmarkHeapPushPop(b *testing.B) {
	h := NewHeap[int](func(a, b int) bool { return a < b }, 1024)
	for i := 0; i < 512; i++ {
		h.Push(rand.Intn(10000))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		h.Push(i)
		_, _ = h.Pop()
	}
}
