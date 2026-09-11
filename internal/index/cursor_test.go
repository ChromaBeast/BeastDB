package index

import (
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

func TestCursorRangeScans(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_cursor.bin")

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk manager: %v", err)
	}
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 50)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	const totalKeys = 350
	for i := 1; i <= totalKeys; i++ {
		k := uint64(i)
		_ = tree.Insert(k, storage.RID{PageID: k, SlotID: uint16(i)})
	}

	// 1. Full scan across multiple pages
	cursor, err := tree.Scan(1, totalKeys)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	defer cursor.Close()

	var collected []uint64
	for {
		k, rid, ok, err := cursor.Next()
		if err != nil {
			t.Fatalf("cursor Next error: %v", err)
		}
		if !ok {
			break
		}
		if rid.PageID != k || rid.SlotID != uint16(k) {
			t.Fatalf("corrupted RID for key %d", k)
		}
		collected = append(collected, k)
	}

	if len(collected) != totalKeys {
		t.Fatalf("expected %d keys, got %d", totalKeys, len(collected))
	}
	for i, k := range collected {
		if k != uint64(i+1) {
			t.Fatalf("at index %d: expected key %d, got %d", i, i+1, k)
		}
	}

	// 2. Sub-range scan: [50, 150]
	subCursor, err := tree.Scan(50, 150)
	if err != nil {
		t.Fatalf("sub scan failed: %v", err)
	}
	defer subCursor.Close()

	var subKeys []uint64
	for {
		k, _, ok, err := subCursor.Next()
		if err != nil || !ok {
			break
		}
		subKeys = append(subKeys, k)
	}

	if len(subKeys) != 101 || subKeys[0] != 50 || subKeys[len(subKeys)-1] != 150 {
		t.Fatalf("sub-range mismatch: length=%d, first=%d, last=%d", len(subKeys), subKeys[0], subKeys[len(subKeys)-1])
	}
}

func BenchmarkCursorScan(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_cursor.bin")

	disk, _ := storage.OpenDiskManager(dbPath)
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 50)
	tree, _ := CreateBPlusTree(bpm)

	const totalKeys = 1000
	for i := 1; i <= totalKeys; i++ {
		k := uint64(i)
		_ = tree.Insert(k, storage.RID{PageID: k, SlotID: uint16(i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor, _ := tree.Scan(100, 200)
		for {
			_, _, ok, _ := cursor.Next()
			if !ok {
				break
			}
		}
		_ = cursor.Close()
	}
}
