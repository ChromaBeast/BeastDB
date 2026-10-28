package api

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

func chainRIDs(t *testing.T, e *Engine, key uint64) []storage.RID {
	t.Helper()
	rid, err := e.tree.Find(key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.readManifest(rid)
	if err != nil {
		t.Fatal(err)
	}
	var out []storage.RID
	if err := e.walkChain(m, func(r storage.RID, _ []byte) { out = append(out, r) }); err != nil {
		t.Fatal(err)
	}
	return out
}

// A mis-linked chain that still has the right length and terminates must not
// free chunks owned by another key: the value's length and CRC must check out first.
func TestFreeRefusesChainThatFailsCRC(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	_ = e.Put(1, pattern(20<<10, 1))
	_ = e.Put(2, pattern(20<<10, 2))
	a, b := chainRIDs(t, e, 1), chainRIDs(t, e, 2)
	page, _ := e.bpm.FetchPage(a[0].PageID)
	tuple, _ := page.GetTuple(a[0].SlotID)
	binary.LittleEndian.PutUint64(tuple[1:9], b[1].PageID)
	binary.LittleEndian.PutUint16(tuple[9:11], b[1].SlotID)
	_ = e.bpm.UnpinPage(a[0].PageID, true)

	if err := e.Delete(1); err != nil {
		t.Fatal(err)
	}
	mustGet(t, e, 2, pattern(20<<10, 2))
}

// The WAL frames each batch operation with 15 bytes; that framing must count
// toward MaxBatchBytes so a committed batch always fits a replication message.
func TestBatchLimitCountsWALFraming(t *testing.T) {
	e := openLarge(t, t.TempDir())
	defer e.Close()
	n := MaxBatchBytes/16 + 1 // one-byte values: fits the old value-only limit
	ops := make([]BatchOperation, n)
	for i := range ops {
		ops[i] = BatchOperation{Type: BatchOpPut, Key: uint64(i), Value: []byte{1}}
	}
	if err := e.BatchWrite(ops); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("expected ErrBatchTooLarge for a batch whose WAL frame exceeds the limit, got %v", err)
	}
}
