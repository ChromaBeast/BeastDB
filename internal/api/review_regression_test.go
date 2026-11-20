package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// crashCopy copies a live database's files to a new directory, which is the
// state a crash at that moment would leave on disk.
func crashCopy(t *testing.T, srcDir string) string {
	t.Helper()
	dst := t.TempDir()
	for _, n := range []string{"d.bin", "d.wal", "d.bin-journal", "d.wal.bak"} {
		in, err := os.Open(filepath.Join(srcDir, n))
		if err != nil {
			continue
		}
		out, err := os.Create(filepath.Join(dst, n))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(out, in); err != nil {
			t.Fatal(err)
		}
		in.Close()
		out.Close()
	}
	return dst
}

func openAt(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := NewEngine(filepath.Join(dir, "d.bin"), filepath.Join(dir, "d.wal"), 64)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return e
}

func mustPut(t *testing.T, e *Engine, k uint64, v string) {
	t.Helper()
	if err := e.Put(k, []byte(v)); err != nil {
		t.Fatalf("put %d: %v", k, err)
	}
}

// Deleting a leaf's first key and then merging that leaf used to leave the
// parent routing to the dead leaf, so scans and point reads disagreed.
func TestScanMatchesGetAfterLeafMerge(t *testing.T) {
	e := openAt(t, t.TempDir())
	defer e.Close()
	for i := uint64(1); i <= 1000; i++ {
		mustPut(t, e, i, fmt.Sprintf("v%d", i))
	}
	if err := e.Delete(101); err != nil {
		t.Fatal(err)
	}
	mustPut(t, e, 150, "NEW")
	if err := e.Delete(160); err != nil {
		t.Fatal(err)
	}

	items, _, more, err := e.ScanChunk(1, 1000, 2000, 1<<30)
	if err != nil || more {
		t.Fatalf("scan: more=%v err=%v", more, err)
	}
	if len(items) != 998 {
		t.Fatalf("scan returned %d records, want 998", len(items))
	}
	for _, it := range items {
		want, found, err := e.Get(it.Key)
		if err != nil || !found || string(want) != string(it.Value) {
			t.Fatalf("key %d: scan=%q get=%q found=%v err=%v", it.Key, it.Value, want, found, err)
		}
		if it.Key == 101 || it.Key == 160 {
			t.Fatalf("scan returned deleted key %d", it.Key)
		}
	}
}

// Heavy interleaved deletes exercise borrow and merge on both sides.
func TestDeleteManyKeepsTreeConsistent(t *testing.T) {
	e := openAt(t, t.TempDir())
	defer e.Close()
	const n = 3000
	for i := uint64(1); i <= n; i++ {
		mustPut(t, e, i, fmt.Sprintf("v%d", i))
	}
	live := map[uint64]bool{}
	for i := uint64(1); i <= n; i++ {
		live[i] = true
	}
	for _, step := range []uint64{3, 2, 5} {
		for i := uint64(1); i <= n; i += step {
			if live[i] {
				if err := e.Delete(i); err != nil {
					t.Fatalf("delete %d: %v", i, err)
				}
				delete(live, i)
			}
		}
	}
	items, _, _, err := e.ScanChunk(0, ^uint64(0), 1<<20, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(live) {
		t.Fatalf("scan returned %d records, want %d", len(items), len(live))
	}
	for i, it := range items {
		if !live[it.Key] || string(it.Value) != fmt.Sprintf("v%d", it.Key) {
			t.Fatalf("unexpected record %d=%q", it.Key, it.Value)
		}
		if i > 0 && items[i-1].Key >= it.Key {
			t.Fatalf("scan out of order at %d", it.Key)
		}
	}
	for k := range live {
		if _, found, err := e.Get(k); err != nil || !found {
			t.Fatalf("live key %d: found=%v err=%v", k, found, err)
		}
	}
}

func TestWriteAfterTornTailSurvivesNextCrash(t *testing.T) {
	d1 := t.TempDir()
	e := openAt(t, d1)
	for i := uint64(1); i <= 3; i++ {
		mustPut(t, e, i, "a")
	}
	d2 := crashCopy(t, d1)
	e.Close()

	torn := wal.EncodeRecord(&wal.Record{LSN: 4, Type: wal.OpPut, Key: make([]byte, 8), Value: []byte("partial-payload")})
	f, err := os.OpenFile(filepath.Join(d2, "d.wal"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(torn[:len(torn)-5])
	f.Close()

	e2 := openAt(t, d2)
	mustPut(t, e2, 4, "acked-after-recovery")
	d3 := crashCopy(t, d2)
	e2.Close()

	e3 := openAt(t, d3)
	defer e3.Close()
	for k := uint64(1); k <= 4; k++ {
		if _, found, err := e3.Get(k); err != nil || !found {
			t.Fatalf("key %d lost after second crash (found=%v err=%v)", k, found, err)
		}
	}
}

// A halted engine must not checkpoint: the next open has to redo the WAL.
func TestHaltedEngineDoesNotCheckpoint(t *testing.T) {
	d1 := t.TempDir()
	e := openAt(t, d1)
	mustPut(t, e, 1, "a")
	e.mu.Lock()
	_, _ = e.wal.Append(wal.OpPut, []byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("logged-not-applied"))
	_ = e.halt(errors.New("simulated apply failure"))
	e.mu.Unlock()

	if err := e.Checkpoint(); err == nil {
		t.Fatalf("Checkpoint on a halted engine must fail")
	}
	if _, err := e.CreateSnapshot(t.TempDir()); err == nil {
		t.Fatalf("CreateSnapshot on a halted engine must fail")
	}
	if err := e.Close(); err == nil {
		t.Fatalf("Close on a halted engine must report the halt")
	}

	e2 := openAt(t, d1)
	defer e2.Close()
	if v, found, _ := e2.Get(2); !found || string(v) != "logged-not-applied" {
		t.Fatalf("logged record was not replayed after halt: found=%v v=%q", found, v)
	}
}

func TestReplicatedBatchIsLoggedLocally(t *testing.T) {
	d1 := t.TempDir()
	e := openAt(t, d1)
	payload, _ := wal.EncodeBatchPayload([]wal.BatchOp{
		{Type: wal.OpPut, Key: []byte{9, 0, 0, 0, 0, 0, 0, 0}, Value: []byte("b")},
		{Type: wal.OpDelete, Key: []byte{77, 0, 0, 0, 0, 0, 0, 0}},
	})
	before := e.CurrentLSN()
	if err := e.ApplyReplicatedRecord(wal.OpBatch, nil, payload); err != nil {
		t.Fatal(err)
	}
	if got := e.CurrentLSN(); got != before+1 {
		t.Fatalf("LSN after replicated batch = %d, want %d", got, before+1)
	}
	d2 := crashCopy(t, d1)
	e.Close()

	e2 := openAt(t, d2)
	defer e2.Close()
	if _, found, _ := e2.Get(9); !found {
		t.Fatalf("replicated batch lost after a crash on the replica")
	}
}

// A blank meta page in front of real data is damage; the file must be kept.
func TestBlankMetaPageDoesNotDeleteData(t *testing.T) {
	d1 := t.TempDir()
	e := openAt(t, d1)
	for i := uint64(1); i <= 300; i++ {
		mustPut(t, e, i, "a")
	}
	_ = e.Checkpoint()
	mustPut(t, e, 301, "a")
	_ = e.Checkpoint()
	e.Close()

	p := filepath.Join(d1, "d.bin")
	before, _ := os.Stat(p)
	f, err := os.OpenFile(p, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteAt(make([]byte, storage.PageSize), 0)
	f.Close()

	if _, err := NewEngine(p, filepath.Join(d1, "d.wal"), 64); !errors.Is(err, ErrMetaPageDamaged) {
		t.Fatalf("open with blank meta page: got %v, want ErrMetaPageDamaged", err)
	}
	after, err := os.Stat(p)
	if err != nil || after.Size() != before.Size() {
		t.Fatalf("data file was modified or removed: %v", err)
	}
}

func TestScanChunkResumes(t *testing.T) {
	e := openAt(t, t.TempDir())
	defer e.Close()
	for i := uint64(1); i <= 25; i++ {
		mustPut(t, e, i, "v")
	}
	var got []uint64
	start := uint64(1)
	for {
		items, next, more, err := e.ScanChunk(start, 20, 7, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			got = append(got, it.Key)
		}
		if !more {
			break
		}
		start = next
	}
	if len(got) != 20 || got[0] != 1 || got[19] != 20 {
		t.Fatalf("chunked scan = %v", got)
	}
	// A cursor is not left open between chunks, so a write must not block.
	mustPut(t, e, 26, "v")
}

func TestGRPCRejectsSystemKeys(t *testing.T) {
	e := openAt(t, t.TempDir())
	defer e.Close()
	sys := uint64(SystemPartition)<<56 | 42
	mustPut(t, e, sys, "secret") // as the server's own stores would
	mustPut(t, e, 1, "user")
	s := &GRPCServer{engine: e}
	ctx := context.Background()

	denied := func(name string, err error) {
		t.Helper()
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s on a system key: got %v, want PermissionDenied", name, err)
		}
	}
	_, err := s.Get(ctx, &beastv1.GetRequest{Key: sys})
	denied("Get", err)
	_, err = s.Put(ctx, &beastv1.PutRequest{Key: sys, Value: []byte("x")})
	denied("Put", err)
	_, err = s.Delete(ctx, &beastv1.DeleteRequest{Key: sys})
	denied("Delete", err)
	_, err = s.BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: []*beastv1.BatchOperation{
		{OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT, Key: sys, Value: []byte("x")},
	}})
	denied("BatchWrite", err)

	if v, _, _ := e.Get(sys); string(v) != "secret" {
		t.Fatalf("system record was modified: %q", v)
	}
}
