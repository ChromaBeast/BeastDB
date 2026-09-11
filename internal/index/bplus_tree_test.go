package index

import (
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

func TestBPlusTreeBasicInsertAndFind(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bplus_basic.bin")

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk manager: %v", err)
	}
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 10)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatalf("failed to create bplus tree: %v", err)
	}

	keys := []uint64{42, 10, 99, 5, 23}
	for _, k := range keys {
		err := tree.Insert(k, storage.RID{PageID: k, SlotID: uint16(k)})
		if err != nil {
			t.Fatalf("failed to insert key %d: %v", k, err)
		}
	}

	for _, k := range keys {
		rid, err := tree.Find(k)
		if err != nil {
			t.Fatalf("failed to find key %d: %v", k, err)
		}
		if rid.PageID != k || rid.SlotID != uint16(k) {
			t.Fatalf("mismatched RID for key %d: %+v", k, rid)
		}
	}

	if _, err := tree.Find(9999); err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestBPlusTreeSplitsAndRootGrowth(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bplus_split.bin")

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}
	defer disk.Close()

	// 50 frames buffer pool
	bpm := storage.NewBufferPoolManager(disk, 50)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	// MaxLeafEntries is 200, so inserting 450 keys will force multiple leaf splits
	// and root growth from leaf into internal node router!
	numKeys := 450
	for i := 1; i <= numKeys; i++ {
		k := uint64(i)
		err := tree.Insert(k, storage.RID{PageID: k * 2, SlotID: uint16(k % 100)})
		if err != nil {
			t.Fatalf("insert failed for key %d: %v", k, err)
		}
	}

	// Verify all keys can be queried correctly
	for i := 1; i <= numKeys; i++ {
		k := uint64(i)
		rid, err := tree.Find(k)
		if err != nil {
			t.Fatalf("failed to find key %d: %v", k, err)
		}
		if rid.PageID != k*2 || rid.SlotID != uint16(k%100) {
			t.Fatalf("mismatched RID for key %d: %+v", k, rid)
		}
	}
}

func TestBPlusTreePersistenceAndReload(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_bplus_persist.bin")

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk: %v", err)
	}

	bpm := storage.NewBufferPoolManager(disk, 50)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	for i := 1; i <= 300; i++ {
		k := uint64(i)
		_ = tree.Insert(k, storage.RID{PageID: k, SlotID: uint16(i)})
	}

	rootID := tree.RootPageID()
	if err := bpm.FlushAll(); err != nil {
		t.Fatalf("failed to flush buffer pool: %v", err)
	}
	_ = disk.Close()

	// Reopen database file and tree
	reopenedDisk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen disk: %v", err)
	}
	defer reopenedDisk.Close()

	reopenedBPM := storage.NewBufferPoolManager(reopenedDisk, 50)
	reopenedTree := OpenBPlusTree(rootID, reopenedBPM)

	for i := 1; i <= 300; i++ {
		k := uint64(i)
		rid, err := reopenedTree.Find(k)
		if err != nil {
			t.Fatalf("failed to find key %d after reopen: %v", k, err)
		}
		if rid.PageID != k || rid.SlotID != uint16(i) {
			t.Fatalf("mismatched RID after reopen for key %d", k)
		}
	}
}

func BenchmarkBPlusTreeFind(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_bplus.bin")

	disk, _ := storage.OpenDiskManager(dbPath)
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 100)
	tree, _ := CreateBPlusTree(bpm)

	const n = 1000
	for i := uint64(1); i <= n; i++ {
		_ = tree.Insert(i, storage.RID{PageID: i, SlotID: uint16(i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := uint64((i % n) + 1)
		_, _ = tree.Find(key)
	}
}

