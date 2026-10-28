package api

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newCondEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	db, wl := filepath.Join(dir, "c.bin"), filepath.Join(dir, "c.wal")
	e, err := NewEngine(db, wl, 50)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e, db, wl
}

func TestPutIfConditions(t *testing.T) {
	e, _, _ := newCondEngine(t)

	if err := e.PutIf(1, []byte("a"), Condition{Kind: CondPresent}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("present on missing key: got %v", err)
	}
	if err := e.PutIf(1, []byte("a"), Condition{Kind: CondAbsent}); err != nil {
		t.Fatalf("absent on missing key: %v", err)
	}
	if err := e.PutIf(1, []byte("b"), Condition{Kind: CondAbsent}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("absent on existing key: got %v", err)
	}
	if err := e.PutIf(1, []byte("b"), Condition{Kind: CondValueEquals, Value: []byte("zzz")}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("value mismatch: got %v", err)
	}
	if err := e.PutIf(1, []byte("b"), Condition{Kind: CondValueEquals, Value: []byte("a")}); err != nil {
		t.Fatalf("value match: %v", err)
	}
	if v, _, _ := e.Get(1); string(v) != "b" {
		t.Fatalf("got %q want b", v)
	}
	if err := e.PutIf(2, []byte("x"), Condition{Kind: CondValueEquals, Value: []byte("a")}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("value-equals on missing key: got %v", err)
	}
	if _, found, _ := e.Get(2); found {
		t.Fatal("failed conditional put must not write")
	}
}

func TestDeleteIf(t *testing.T) {
	e, _, _ := newCondEngine(t)
	_ = e.Put(1, []byte("a"))
	if err := e.DeleteIf(1, Condition{Kind: CondValueEquals, Value: []byte("no")}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("got %v", err)
	}
	if _, found, _ := e.Get(1); !found {
		t.Fatal("record deleted despite failed condition")
	}
	if err := e.DeleteIf(1, Condition{Kind: CondValueEquals, Value: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteIf(1, Condition{Kind: CondPresent}); !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("delete of missing key with present condition: got %v", err)
	}
}

func TestBatchWriteIfIsAllOrNothing(t *testing.T) {
	e, _, _ := newCondEngine(t)
	_ = e.Put(1, []byte("old"))

	ops := []BatchOperation{
		{Type: BatchOpPut, Key: 2, Value: []byte("new")},
		{Type: BatchOpDelete, Key: 1},
	}
	err := e.BatchWriteIf(ops, []KeyCondition{
		{Key: 1, Cond: Condition{Kind: CondValueEquals, Value: []byte("old")}},
		{Key: 2, Cond: Condition{Kind: CondPresent}}, // fails
	})
	if !errors.Is(err, ErrConditionFailed) {
		t.Fatalf("got %v", err)
	}
	if _, found, _ := e.Get(2); found {
		t.Fatal("partial batch applied")
	}
	if _, found, _ := e.Get(1); !found {
		t.Fatal("delete applied despite failed condition")
	}

	err = e.BatchWriteIf(ops, []KeyCondition{
		{Key: 1, Cond: Condition{Kind: CondValueEquals, Value: []byte("old")}},
		{Key: 2, Cond: Condition{Kind: CondAbsent}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := e.Get(1); found {
		t.Fatal("key 1 should be deleted")
	}
	if v, _, _ := e.Get(2); string(v) != "new" {
		t.Fatalf("key 2 = %q", v)
	}
}

// Many writers increment one counter with compare-and-swap; no update may be lost.
func TestConcurrentCompareAndSwapCounter(t *testing.T) {
	e, _, _ := newCondEngine(t)
	_ = e.Put(7, []byte("0"))

	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				for {
					cur, _, err := e.Get(7)
					if err != nil {
						t.Error(err)
						return
					}
					n, _ := strconv.Atoi(string(cur))
					err = e.PutIf(7, []byte(strconv.Itoa(n+1)), Condition{Kind: CondValueEquals, Value: cur})
					if err == nil {
						break
					}
					if !errors.Is(err, ErrConditionFailed) {
						t.Error(err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	v, _, _ := e.Get(7)
	if string(v) != strconv.Itoa(workers*perWorker) {
		t.Fatalf("lost updates: counter = %s, want %d", v, workers*perWorker)
	}
}

// Conditional writes log only their effect; reopening must reproduce the state.
func TestConditionalWritesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	db, wl := filepath.Join(dir, "r.bin"), filepath.Join(dir, "r.wal")
	e, err := NewEngine(db, wl, 50)
	if err != nil {
		t.Fatal(err)
	}
	_ = e.PutIf(1, []byte("a"), Condition{Kind: CondAbsent})
	_ = e.PutIf(1, []byte("b"), Condition{Kind: CondValueEquals, Value: []byte("a")})
	_ = e.PutIf(1, []byte("c"), Condition{Kind: CondValueEquals, Value: []byte("a")}) // fails
	_ = e.BatchWriteIf([]BatchOperation{{Type: BatchOpPut, Key: 2, Value: []byte("z")}},
		[]KeyCondition{{Key: 1, Cond: Condition{Kind: CondValueEquals, Value: []byte("b")}}})
	_ = e.Close()

	e2, err := NewEngine(db, wl, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if v, _, _ := e2.Get(1); string(v) != "b" {
		t.Fatalf("key 1 = %q after restart", v)
	}
	if v, _, _ := e2.Get(2); string(v) != "z" {
		t.Fatalf("key 2 = %q after restart", v)
	}
}

func TestGRPCConditionalAndScanLimit(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()
	ctx := context.Background()

	absent := &beastv1.Condition{Kind: beastv1.ConditionKind_CONDITION_KIND_ABSENT}
	if _, err := client.Put(ctx, &beastv1.PutRequest{Key: 5, Value: []byte("v1"), Condition: absent}); err != nil {
		t.Fatal(err)
	}
	_, err := client.Put(ctx, &beastv1.PutRequest{Key: 5, Value: []byte("v2"), Condition: absent})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("want Aborted, got %v", err)
	}
	eq := &beastv1.Condition{Kind: beastv1.ConditionKind_CONDITION_KIND_VALUE_EQUALS, Value: []byte("v1")}
	if _, err := client.Delete(ctx, &beastv1.DeleteRequest{Key: 5, Condition: &beastv1.Condition{
		Kind: beastv1.ConditionKind_CONDITION_KIND_VALUE_EQUALS, Value: []byte("nope")}}); status.Code(err) != codes.Aborted {
		t.Fatalf("want Aborted, got %v", err)
	}
	if _, err := client.Put(ctx, &beastv1.PutRequest{Key: 5, Value: []byte("v2"), Condition: eq}); err != nil {
		t.Fatal(err)
	}

	_, err = client.BatchWrite(ctx, &beastv1.BatchWriteRequest{
		Operations: []*beastv1.BatchOperation{{OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT, Key: 6, Value: []byte("x")}},
		Conditions: []*beastv1.KeyCondition{{Key: 5, Condition: eq}}, // value is now v2
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("want Aborted, got %v", err)
	}
	if r, _ := client.Get(ctx, &beastv1.GetRequest{Key: 6}); r.Found {
		t.Fatal("batch applied despite failed condition")
	}

	// Scan limit and resume.
	for k := uint64(100); k < 110; k++ {
		if _, err := client.Put(ctx, &beastv1.PutRequest{Key: k, Value: []byte("s")}); err != nil {
			t.Fatal(err)
		}
	}
	var got []uint64
	start := uint64(100)
	for {
		stream, err := client.Scan(ctx, &beastv1.ScanRequest{StartKey: start, EndKey: 109, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for {
			kv, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, kv.Key)
			start = kv.Key + 1
			n++
		}
		if n == 0 {
			break
		}
		if n > 4 {
			t.Fatalf("page returned %d > limit", n)
		}
	}
	if len(got) != 10 {
		t.Fatalf("paged scan returned %v", got)
	}
}
