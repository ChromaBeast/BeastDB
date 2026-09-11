package index

import (
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

func TestLeafNodeOperations(t *testing.T) {
	buf := make([]byte, storage.PageSize)
	leaf := InitLeafNode(buf, true, 0)

	// Insert out of order
	keys := []uint64{30, 10, 50, 20, 40}
	for _, k := range keys {
		leaf.Insert(k, storage.RID{PageID: k * 10, SlotID: uint16(k)})
	}

	if leaf.Header().KeyCount != 5 {
		t.Fatalf("expected 5 keys, got %d", leaf.Header().KeyCount)
	}

	// Verify sorted order
	expected := []uint64{10, 20, 30, 40, 50}
	for i, exp := range expected {
		if leaf.Key(i) != exp {
			t.Fatalf("at index %d: expected key %d, got %d", i, exp, leaf.Key(i))
		}
		rid, found := leaf.Lookup(exp)
		if !found || rid.PageID != exp*10 || rid.SlotID != uint16(exp) {
			t.Fatalf("failed to lookup key %d: found=%v, rid=%+v", exp, found, rid)
		}
	}

	// Test non-existent key
	if _, found := leaf.Lookup(999); found {
		t.Fatalf("expected non-existent key to not be found")
	}

	// Test leaf split
	rightBuf := make([]byte, storage.PageSize)
	splitKey, rightLeaf := leaf.Split(rightBuf, 99)

	if splitKey != 30 {
		t.Fatalf("expected split key 30, got %d", splitKey)
	}
	if leaf.Header().KeyCount != 2 || rightLeaf.Header().KeyCount != 3 {
		t.Fatalf("unexpected counts: left=%d, right=%d", leaf.Header().KeyCount, rightLeaf.Header().KeyCount)
	}
	if leaf.Header().NextPageID != 99 {
		t.Fatalf("expected left leaf NextPageID to be 99, got %d", leaf.Header().NextPageID)
	}
}

func TestInternalNodeOperations(t *testing.T) {
	buf := make([]byte, storage.PageSize)
	internal := InitInternalNode(buf, true, 0, 100)

	internal.Insert(20, 101)
	internal.Insert(40, 102)
	internal.Insert(60, 103)

	tests := []struct {
		searchKey uint64
		expected  uint64
	}{
		{5, 100},
		{20, 101},
		{35, 101},
		{40, 102},
		{59, 102},
		{60, 103},
		{1000, 103},
	}

	for _, tc := range tests {
		got := internal.Lookup(tc.searchKey)
		if got != tc.expected {
			t.Fatalf("Lookup(%d): expected child %d, got %d", tc.searchKey, tc.expected, got)
		}
	}
}
