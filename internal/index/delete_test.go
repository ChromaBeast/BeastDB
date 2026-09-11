package index

import (
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

func TestLeafNodeDelete(t *testing.T) {
	buf := make([]byte, storage.PageSize)
	leaf := InitLeafNode(buf, true, 0)

	keys := []uint64{10, 20, 30, 40, 50}
	for _, k := range keys {
		leaf.Insert(k, storage.RID{PageID: k, SlotID: uint16(k)})
	}

	if !leaf.Delete(30) {
		t.Fatalf("expected delete of 30 to succeed")
	}
	if leaf.Header().KeyCount != 4 {
		t.Fatalf("expected key count 4, got %d", leaf.Header().KeyCount)
	}

	if _, found := leaf.Lookup(30); found {
		t.Fatalf("deleted key 30 still found")
	}

	if leaf.Delete(999) {
		t.Fatalf("expected delete of non-existent key to return false")
	}
}

func TestBPlusTreeDeleteAndUnderflow(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_delete.bin")

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 50)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	// Insert 250 keys (triggers leaf split)
	for i := 1; i <= 250; i++ {
		k := uint64(i)
		_ = tree.Insert(k, storage.RID{PageID: k, SlotID: uint16(i)})
	}

	// Delete key 15
	if err := tree.Delete(15); err != nil {
		t.Fatalf("failed to delete key 15: %v", err)
	}
	if _, err := tree.Find(15); err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound for key 15, got %v", err)
	}

	// Delete non-existent key
	if err := tree.Delete(9999); err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound for key 9999, got %v", err)
	}

	// Delete 40 keys to exercise underflow borrowing & merging
	for i := 1; i <= 40; i++ {
		_ = tree.Delete(uint64(i))
	}

	// Ensure all remaining keys [41, 250] exist
	for i := 41; i <= 250; i++ {
		k := uint64(i)
		rid, err := tree.Find(k)
		if err != nil {
			t.Fatalf("failed to find remaining key %d: %v", k, err)
		}
		if rid.PageID != k {
			t.Fatalf("mismatched RID for key %d", k)
		}
	}
}
