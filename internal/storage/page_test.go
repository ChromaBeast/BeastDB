package storage

import (
	"bytes"
	"testing"
)

func TestSlottedPageInsertAndGet(t *testing.T) {
	page := NewSlottedPage(1)

	// Insert variable-length records
	id0, err := page.InsertTuple([]byte("Alice"))
	if err != nil || id0 != 0 {
		t.Fatalf("failed to insert Alice: id=%d, err=%v", id0, err)
	}

	id1, err := page.InsertTuple([]byte("Bob"))
	if err != nil || id1 != 1 {
		t.Fatalf("failed to insert Bob: id=%d, err=%v", id1, err)
	}

	id2, err := page.InsertTuple([]byte("A very long database record string payload for testing"))
	if err != nil || id2 != 2 {
		t.Fatalf("failed to insert long record: id=%d, err=%v", id2, err)
	}

	// Retrieve records by SlotID
	rec0, err := page.GetTuple(0)
	if err != nil || string(rec0) != "Alice" {
		t.Fatalf("unexpected rec0: %q, err=%v", string(rec0), err)
	}

	rec1, err := page.GetTuple(1)
	if err != nil || string(rec1) != "Bob" {
		t.Fatalf("unexpected rec1: %q, err=%v", string(rec1), err)
	}

	rec2, err := page.GetTuple(2)
	if err != nil || !bytes.Contains(rec2, []byte("database record string")) {
		t.Fatalf("unexpected rec2: %q, err=%v", string(rec2), err)
	}
}

func TestSlottedPageDeleteAndCompact(t *testing.T) {
	page := NewSlottedPage(42)

	// Insert 3 records
	s0, _ := page.InsertTuple([]byte("Record-Zero"))
	s1, _ := page.InsertTuple([]byte("Record-One-To-Be-Deleted"))
	s2, _ := page.InsertTuple([]byte("Record-Two"))

	freeBeforeDelete := page.FreeSpace()

	// Delete s1 (leaves gap in tuple area)
	if err := page.DeleteTuple(s1); err != nil {
		t.Fatalf("failed to delete s1: %v", err)
	}

	// s1 must now return ErrTupleDeleted
	if _, err := page.GetTuple(s1); err != ErrTupleDeleted {
		t.Fatalf("expected ErrTupleDeleted, got %v", err)
	}

	// Defragment the page
	page.Compact()

	freeAfterCompact := page.FreeSpace()
	if freeAfterCompact <= freeBeforeDelete {
		t.Fatalf("expected free space to increase after compact: before=%d, after=%d",
			freeBeforeDelete, freeAfterCompact)
	}

	// s0 and s2 must remain fully intact at their original SlotIDs!
	rec0, err := page.GetTuple(s0)
	if err != nil || string(rec0) != "Record-Zero" {
		t.Fatalf("corrupted s0 after compact: %s", string(rec0))
	}

	rec2, err := page.GetTuple(s2)
	if err != nil || string(rec2) != "Record-Two" {
		t.Fatalf("corrupted s2 after compact: %s", string(rec2))
	}
}

func TestSlottedPageCapacityLimit(t *testing.T) {
	page := NewSlottedPage(10)
	chunk := make([]byte, 500) // 500-byte record

	inserted := 0
	for {
		_, err := page.InsertTuple(chunk)
		if err == ErrPageFull {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		inserted++
	}

	// 4096 / (500 + 4) = ~8 records max
	if inserted < 7 || inserted > 8 {
		t.Fatalf("expected 7 or 8 records to fit in 4KB page, got %d", inserted)
	}
}

func BenchmarkSlottedPageGet(b *testing.B) {
	page := NewSlottedPage(1)
	slotID, _ := page.InsertTuple([]byte("benchmark:payload:tuple:data"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = page.GetTuple(slotID)
	}
}
