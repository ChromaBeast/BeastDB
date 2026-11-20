package api

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/index"
)

func waitReady(t *testing.T, e *Engine, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		infos, err := e.ListIndexes()
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range infos {
			if i.Name == name && i.State == IndexReady {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("index %q never became ready", name)
}

func userDoc(name string, age int) []byte {
	return []byte(fmt.Sprintf(`{"name":%q,"age":%d,"addr":{"city":"c%d"}}`, name, age, age%3))
}

func matchKeys(t *testing.T, e *Engine, f IndexFilter) []uint64 {
	t.Helper()
	items, _, err := e.ScanIndexed(f, 0, ^uint64(0), 0)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]uint64, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	return keys
}

// putMany writes keys from..to (inclusive, local) in batches, one fsync per batch.
func putMany(t *testing.T, e *Engine, prefix uint8, from, to uint64, doc func(n uint64) []byte) {
	t.Helper()
	const per = 200
	for lo := from; lo <= to; lo += per {
		var ops []BatchOperation
		for n := lo; n < lo+per && n <= to; n++ {
			k, _ := CollectionKey(prefix, n)
			ops = append(ops, BatchOperation{Type: BatchOpPut, Key: k, Value: doc(n)})
		}
		if err := e.BatchWrite(ops); err != nil {
			t.Fatal(err)
		}
	}
}

func i64(v int64) *IndexValue  { return &IndexValue{Int: v} }
func str(v string) *IndexValue { return &IndexValue{IsString: true, Str: v} }

func TestIndexCodecStrict(t *testing.T) {
	in := []IndexDef{{"a", 0x10, "age", index.KindInt, 5}, {"b", 0x11, "x.y", index.KindString, 6}}
	out, err := decodeIndexCatalog(encodeIndexCatalog(in))
	if err != nil || len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round trip: %v %v", out, err)
	}
	raw := encodeIndexCatalog(in)
	for _, bad := range [][]byte{nil, raw[:3], raw[:len(raw)-1], append(append([]byte{}, raw...), 0), append([]byte("XXXX"), raw[4:]...)} {
		if _, err := decodeIndexCatalog(bad); !errors.Is(err, ErrCorruptIndexCatalog) {
			t.Errorf("expected corrupt for %v, got %v", bad, err)
		}
	}
}

func TestIndexLifecycleBuildMaintainAndRestart(t *testing.T) {
	dir := t.TempDir()
	e := openCollEngine(t, dir)
	if _, err := e.CreateCollection("users", nil); err != nil {
		t.Fatal(err)
	}
	cols, _ := e.ListCollections()
	p := cols[0].Prefix
	key := func(n uint64) uint64 { k, _ := CollectionKey(p, n); return k }

	// Records that exist before the index is created, including unindexable ones.
	putMany(t, e, p, 1, 3000, func(n uint64) []byte { return userDoc(fmt.Sprintf("u%d", n), int(n%100)) })
	if err := e.Put(key(5000), []byte("not json")); err != nil {
		t.Fatal(err)
	}
	if err := e.Put(key(5001), []byte(`{"age":"old"}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := e.CreateIndex("by-age", "nope", "age", index.KindInt); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("missing collection: %v", err)
	}
	if _, err := e.CreateIndex("by-age", "users", "a b", index.KindInt); !errors.Is(err, ErrInvalidIndex) {
		t.Fatalf("bad path: %v", err)
	}
	if _, err := e.CreateIndex("by-age", "users", "age", index.KindInt); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateIndex("by-age", "users", "age", index.KindInt); !errors.Is(err, ErrIndexExists) {
		t.Fatalf("dup: %v", err)
	}
	if err := e.DropCollection("users"); !errors.Is(err, ErrCollectionNotEmpty) && !errors.Is(err, ErrCollectionHasIndexes) {
		t.Fatalf("drop with data/index: %v", err)
	}
	waitReady(t, e, "by-age")

	got := matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(7)})
	if len(got) != 30 {
		t.Fatalf("age=7 matched %d, want 30", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatal("result not in key order")
		}
	}
	if n := len(matchKeys(t, e, IndexFilter{Index: "by-age", Gte: i64(10), Lte: i64(19)})); n != 300 {
		t.Fatalf("range matched %d, want 300", n)
	}
	infos, _ := e.ListIndexes()
	if infos[0].Entries != 3000 || infos[0].Skipped != 2 || infos[0].Collection != "users" {
		t.Fatalf("info %+v", infos[0])
	}

	// Live maintenance: update, change type to unindexable, delete.
	_ = e.Put(key(1), userDoc("u1", 7))
	_ = e.Put(key(2), []byte(`{"age":"seven"}`))
	_ = e.Delete(key(3))
	if n := len(matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(7)})); n != 31 {
		t.Fatalf("after update age=7 -> %d, want 31", n)
	}
	if n := len(matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(2)})); n != 29 { // key 2 became unindexable
		t.Fatalf("age=2 -> %d, want 29", n)
	}
	if _, _, err := e.ScanIndexed(IndexFilter{Index: "by-age", Eq: str("x")}, 0, ^uint64(0), 0); !errors.Is(err, ErrIndexFilter) {
		t.Fatalf("type mismatch: %v", err)
	}
	if _, _, err := e.ScanIndexed(IndexFilter{Index: "nope", Eq: i64(1)}, 0, ^uint64(0), 0); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("missing: %v", err)
	}

	// Pagination: limit with more=true, resume at last key + 1.
	var all []uint64
	start := uint64(0)
	for {
		items, more, err := e.ScanIndexed(IndexFilter{Index: "by-age", Eq: i64(7)}, start, ^uint64(0), 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			all = append(all, it.Key)
		}
		if !more {
			break
		}
		start = items[len(items)-1].Key + 1
	}
	if len(all) != 31 {
		t.Fatalf("paged %d, want 31", len(all))
	}

	// Clean restart: the index is rebuilt from the records.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openCollEngine(t, dir)
	waitReady(t, e, "by-age")
	if n := len(matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(7)})); n != 31 {
		t.Fatalf("after restart age=7 -> %d, want 31", n)
	}

	// Drop frees the index and then the collection can be emptied and dropped.
	if err := e.DropIndex("by-age"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.ScanIndexed(IndexFilter{Index: "by-age", Eq: i64(7)}, 0, ^uint64(0), 0); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("after drop: %v", err)
	}
	if err := e.DropIndex("by-age"); !errors.Is(err, ErrIndexNotFound) {
		t.Fatalf("double drop: %v", err)
	}
	if infos, _ := e.ListIndexes(); len(infos) != 0 {
		t.Fatalf("still listed: %+v", infos)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
}

func copyFileT(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

// A crash leaves the data file at its last checkpoint and the WAL ahead of it.
// The index definition and later writes come back from the WAL and the index
// is rebuilt; nothing about the index was in the data file.
func TestIndexSurvivesCrashViaWAL(t *testing.T) {
	dir := t.TempDir()
	e := openCollEngine(t, dir)
	if _, err := e.CreateCollection("users", nil); err != nil {
		t.Fatal(err)
	}
	cols, _ := e.ListCollections()
	p := cols[0].Prefix
	putMany(t, e, p, 1, 50, func(n uint64) []byte { return userDoc("u", int(n%5)) })
	if err := e.Checkpoint(); err != nil { // checkpoint BEFORE the index exists
		t.Fatal(err)
	}
	if _, err := e.CreateIndex("by-age", "users", "age", index.KindInt); err != nil {
		t.Fatal(err)
	}
	waitReady(t, e, "by-age")
	k, _ := CollectionKey(p, 99)
	_ = e.Put(k, userDoc("late", 3))

	crash := t.TempDir()
	for _, f := range []string{"db.bin", "db.wal"} {
		copyFileT(t, filepath.Join(dir, f), filepath.Join(crash, f))
	}
	e2 := openCollEngine(t, crash)
	defer e2.Close()
	waitReady(t, e2, "by-age")
	if n := len(matchKeys(t, e2, IndexFilter{Index: "by-age", Eq: i64(3)})); n != 11 {
		t.Fatalf("after crash age=3 -> %d, want 11", n)
	}
	_ = e.Close()
}

func TestIndexConcurrentWritesDuringBuild(t *testing.T) {
	e := openCollEngine(t, t.TempDir())
	defer e.Close()
	if _, err := e.CreateCollection("c", nil); err != nil {
		t.Fatal(err)
	}
	cols, _ := e.ListCollections()
	p := cols[0].Prefix
	const n = 6000
	putMany(t, e, p, 1, n, func(uint64) []byte { return userDoc("u", 1) })
	if _, err := e.CreateIndex("by-age", "c", "age", index.KindInt); err != nil {
		t.Fatal(err)
	}
	// While it builds, rewrite every record to age=2 in reverse order.
	for hi := uint64(n); hi >= 1; hi -= 100 {
		putMany(t, e, p, hi-99, hi, func(uint64) []byte { return userDoc("u", 2) })
	}
	waitReady(t, e, "by-age")
	if c := len(matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(1)})); c != 0 {
		t.Fatalf("stale entries: %d", c)
	}
	if c := len(matchKeys(t, e, IndexFilter{Index: "by-age", Eq: i64(2)})); c != n {
		t.Fatalf("age=2 -> %d, want %d", c, n)
	}
}

func TestIndexCatalogKeyIsReserved(t *testing.T) {
	if !IsReservedKey(IndexCatalogKey) || !IsReservedKey(CatalogKey) || IsReservedKey(1) {
		t.Fatal("IsReservedKey")
	}
}
