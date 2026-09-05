package dsa

import (
	"testing"
)

func TestVectorOperations(t *testing.T) {
	vec := NewVector[int](2)
	if vec.Len() != 0 || vec.Cap() != 2 {
		t.Fatalf("expected len=0, cap=2, got len=%d, cap=%d", vec.Len(), vec.Cap())
	}

	// Test Push and automatic growth
	for i := 1; i <= 10; i++ {
		vec.Push(i)
	}

	if vec.Len() != 10 {
		t.Fatalf("expected len=10, got %d", vec.Len())
	}

	// Test Get and Set
	val, err := vec.Get(4)
	if err != nil || val != 5 {
		t.Fatalf("expected 5 at index 4, got %d (err: %v)", val, err)
	}

	err = vec.Set(4, 99)
	if err != nil {
		t.Fatalf("failed to set value: %v", err)
	}
	val, _ = vec.Get(4)
	if val != 99 {
		t.Fatalf("expected 99 at index 4, got %d", val)
	}

	// Test Pop
	last, err := vec.Pop()
	if err != nil || last != 10 {
		t.Fatalf("expected last element 10, got %d (err: %v)", last, err)
	}
	if vec.Len() != 9 {
		t.Fatalf("expected len=9 after pop, got %d", vec.Len())
	}

	// Test Bounds
	_, err = vec.Get(-1)
	if err != ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds, got %v", err)
	}
	_, err = vec.Get(100)
	if err != ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds, got %v", err)
	}

	// Test ShrinkToFit
	vec.ShrinkToFit()
	if vec.Cap() != vec.Len() {
		t.Fatalf("expected cap=%d after shrink, got %d", vec.Len(), vec.Cap())
	}

	// Test Clear
	vec.Clear()
	if vec.Len() != 0 {
		t.Fatalf("expected len=0 after clear, got %d", vec.Len())
	}
	_, err = vec.Pop()
	if err != ErrEmptyVector {
		t.Fatalf("expected ErrEmptyVector, got %v", err)
	}
}

func BenchmarkVectorPush(b *testing.B) {
	vec := NewVector[int](1024)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vec.Push(i)
		if vec.Len() >= 100000 {
			vec.Clear()
		}
	}
}
