package index

import (
	"math/rand"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Random inserts and deletes against a map: every lookup and full scan must
// agree with the map, through splits, borrows, merges, collapses and page reuse.
func TestRandomInsertDeleteMatchesModel(t *testing.T) {
	disk, err := storage.OpenDiskManager(filepath.Join(t.TempDir(), "r.bin"))
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	bpm := storage.NewBufferPoolManager(disk, 64)
	_, meta, _ := bpm.NewPage() // page 0 is never a tree page
	_ = bpm.UnpinPage(meta, true)
	tree, err := CreateBPlusTree(bpm)
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(42))
	model := map[uint64]storage.RID{}
	check := func(step int) {
		cur, err := tree.Scan(0, ^uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		var keys []uint64
		for {
			k, rid, ok, err := cur.Next()
			if err != nil {
				t.Fatalf("step %d: scan: %v", step, err)
			}
			if !ok {
				break
			}
			if model[k] != rid {
				t.Fatalf("step %d: scan key %d rid %v, model %v", step, k, rid, model[k])
			}
			keys = append(keys, k)
		}
		if len(keys) != len(model) || !sort.SliceIsSorted(keys, func(i, j int) bool { return keys[i] < keys[j] }) {
			t.Fatalf("step %d: scan returned %d keys, model has %d", step, len(keys), len(model))
		}
	}
	for step := 0; step < 40000; step++ {
		k := uint64(rng.Intn(6000))
		if rng.Intn(100) < 55 {
			rid := storage.RID{PageID: uint64(step), SlotID: uint16(step)}
			if err := tree.Insert(k, rid); err != nil {
				t.Fatal(err)
			}
			model[k] = rid
		} else {
			err := tree.Delete(k)
			if _, ok := model[k]; ok != (err == nil) {
				t.Fatalf("step %d: delete %d: err=%v, in model=%v", step, k, err, ok)
			}
			delete(model, k)
		}
		if step%5000 == 0 {
			check(step)
		}
	}
	check(-1)
	for k := range model {
		if err := tree.Delete(k); err != nil {
			t.Fatal(err)
		}
	}
	model = map[uint64]storage.RID{}
	check(-2)
	if bpm.FreeListHead() == 0 {
		t.Fatal("emptying the tree freed no pages")
	}
}
