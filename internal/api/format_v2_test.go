package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

func readMetaFile(t *testing.T, dbPath string) storage.MetaData {
	t.Helper()
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := storage.DecodeMeta(raw[:storage.PageSize])
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// applyRange writes keys from..to (step) in batches: one fsync per batch keeps
// these tests fast. A nil val deletes.
func applyRange(t *testing.T, e *Engine, from, to, step uint64, val func(uint64) string) {
	t.Helper()
	var ops []BatchOperation
	flush := func() {
		if len(ops) > 0 {
			if err := e.BatchWrite(ops); err != nil {
				t.Fatal(err)
			}
			ops = ops[:0]
		}
	}
	for k := from; k <= to; k += step {
		if val == nil {
			ops = append(ops, BatchOperation{Type: BatchOpDelete, Key: k})
		} else {
			ops = append(ops, BatchOperation{Type: BatchOpPut, Key: k, Value: []byte(val(k))})
		}
		if len(ops) == 500 {
			flush()
		}
	}
	flush()
}

// Deleting and re-adding the same data must reuse pages, not grow the file.
func TestChurnReusesPages(t *testing.T) {
	dir := t.TempDir()
	e := openAt(t, dir)
	defer e.Close()
	fill := func(tag string) {
		applyRange(t, e, 1, 3000, 1, func(i uint64) string {
			return fmt.Sprintf("%s-%04d-%s", tag, i, bytes.Repeat([]byte("x"), 60))
		})
	}
	drain := func() { applyRange(t, e, 1, 3000, 1, nil) }
	fill("a")
	grown := e.disk.NumPages()
	for round := 0; round < 3; round++ {
		drain()
		fill(fmt.Sprintf("r%d", round))
	}
	if n := e.disk.NumPages(); n > grown+grown/10 {
		t.Fatalf("file grew from %d to %d pages across delete/refill rounds", grown, n)
	}
	if v, _, _ := e.Get(1500); !bytes.HasPrefix(v, []byte("r2-1500")) {
		t.Fatalf("value after churn = %q", v)
	}
}

// Removing every key collapses the tree back to a single root leaf and frees the rest.
func TestDeleteAllCollapsesTree(t *testing.T) {
	e := openAt(t, t.TempDir())
	defer e.Close()
	applyRange(t, e, 1, 5000, 1, func(uint64) string { return "v" })
	applyRange(t, e, 1, 5000, 1, nil)
	root, err := e.bpm.FetchPage(e.tree.RootPageID())
	if err != nil {
		t.Fatal(err)
	}
	h := index.ReadNodeHeader(root.Data())
	_ = e.bpm.UnpinPage(e.tree.RootPageID(), false)
	if h.NodeType != index.NodeTypeLeaf || !h.IsRoot {
		t.Fatalf("root after deleting everything: %+v", h)
	}
	if e.bpm.FreeListHead() == 0 {
		t.Fatal("no pages were freed")
	}
	mustPut(t, e, 7, "back")
	if v, ok, _ := e.Get(7); !ok || string(v) != "back" {
		t.Fatalf("tree unusable after collapse")
	}
}

func TestFreeSpaceMapSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	e := openAt(t, dir)
	val := func(i uint64) string { return fmt.Sprintf("value-%04d-%s", i, bytes.Repeat([]byte("y"), 40)) }
	applyRange(t, e, 1, 2000, 1, val)
	applyRange(t, e, 1, 2000, 3, nil) // leave every page partly used
	tracked := len(e.reclaim)
	if tracked == 0 {
		t.Fatal("no pages tracked as having free space")
	}
	e.Close()
	if readMetaFile(t, filepath.Join(dir, "d.bin")).SpaceMapHead == 0 {
		t.Fatal("free-space map not saved")
	}

	e2 := openAt(t, dir)
	defer e2.Close()
	if len(e2.reclaim) != tracked {
		t.Fatalf("free-space map after restart: %d pages, want %d", len(e2.reclaim), tracked)
	}
	before := e2.disk.NumPages()
	applyRange(t, e2, 1, 2000, 3, val)
	if after := e2.disk.NumPages(); after > before+2 {
		t.Fatalf("re-inserting freed records grew the file from %d to %d pages", before, after)
	}
}

func TestSearchIndexRestoredAfterCleanShutdown(t *testing.T) {
	dir := t.TempDir()
	e := openAt(t, dir)
	mustPut(t, e, 1, "the quick brown fox")
	mustPut(t, e, 2, "lazy brown dog")
	mustPut(t, e, 3, "unrelated text")
	e.Close()

	m := readMetaFile(t, filepath.Join(dir, "d.bin"))
	if m.IndexSnapHead == 0 || m.IndexSnapLSN != m.LastCheckpointLSN {
		t.Fatalf("search index not saved at Close: %+v", m)
	}

	e2 := openAt(t, dir)
	res, err := e2.Search("brown", 10, 0)
	if err != nil || len(res) != 2 {
		t.Fatalf("search after restart: %v %v", res, err)
	}
	mustPut(t, e2, 4, "brown bear")
	if err := e2.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	// After a crash the saved index is out of date and must be rebuilt, not trusted.
	crashed := crashCopy(t, dir)
	e2.Close()
	if readMetaFile(t, filepath.Join(crashed, "d.bin")).IndexSnapHead != 0 {
		t.Fatal("a periodic checkpoint must not keep an out-of-date index snapshot")
	}
	e3 := openAt(t, crashed)
	defer e3.Close()
	if res, _ := e3.Search("brown", 10, 0); len(res) != 3 {
		t.Fatalf("search after crash = %d results, want 3", len(res))
	}
}

func TestDamagedDataPageIsAnErrorNotData(t *testing.T) {
	dir := t.TempDir()
	e := openAt(t, dir)
	mustPut(t, e, 1, "important-value")
	e.Close()

	p := filepath.Join(dir, "d.bin")
	raw, _ := os.ReadFile(p)
	at := bytes.Index(raw, []byte("important-value"))
	if at < 0 {
		t.Fatal("value not found in data file")
	}
	raw[at] = 'X'
	_ = os.WriteFile(p, raw, 0600)

	e2 := openAt(t, dir)
	defer e2.Close()
	if _, _, err := e2.Get(1); !errors.Is(err, storage.ErrPageChecksum) {
		t.Fatalf("Get on a damaged page: got %v, want ErrPageChecksum", err)
	}
}

func TestMidLogWALDamageFailsOpen(t *testing.T) {
	dir := t.TempDir()
	e := openAt(t, dir)
	for i := uint64(1); i <= 3; i++ {
		mustPut(t, e, i, "v")
	}
	crashed := crashCopy(t, dir)
	e.Close()

	walPath := filepath.Join(crashed, "d.wal")
	raw, _ := os.ReadFile(walPath)
	first := int(binary.LittleEndian.Uint32(raw[0:4]))
	raw[first+wal.HeaderSize] ^= 0xff
	_ = os.WriteFile(walPath, raw, 0600)

	dbPath := filepath.Join(crashed, "d.bin")
	if _, err := NewEngine(dbPath, walPath, 64); !errors.Is(err, wal.ErrCorruptLog) {
		t.Fatalf("open with mid-log damage: got %v, want wal.ErrCorruptLog", err)
	}
	e2, err := NewEngineWithOptions(dbPath, walPath, 64, Options{TruncateCorruptWAL: true})
	if err != nil {
		t.Fatalf("open with truncation allowed: %v", err)
	}
	defer e2.Close()
	if _, ok, _ := e2.Get(1); !ok {
		t.Fatal("the record before the damage was lost")
	}
}

// createV1Database writes a format version 1 database: no page checksums,
// a tuple that ends at byte 4096, a large chained value, and a WAL tail.
func createV1Database(t *testing.T, dir string) (dbPath, walPath string) {
	t.Helper()
	dbPath, walPath = filepath.Join(dir, "v1.bin"), filepath.Join(dir, "v1.wal")
	disk, err := storage.OpenDiskManagerWithoutChecksums(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	bpm := storage.NewBufferPoolManager(disk, 64)
	metaPage, metaID, _ := bpm.NewPage()
	tree, err := index.CreateBPlusTree(bpm)
	if err != nil {
		t.Fatal(err)
	}
	dataPage, dataID, _ := bpm.NewPage()
	// Version 1 pages used all 4096 bytes.
	storage.WritePageHeader(dataPage.Data(), storage.PageHeader{PageID: dataID, Lower: storage.PageHeaderSize, Upper: storage.PageSize})
	_ = bpm.UnpinPage(dataID, true)

	shell := &Engine{disk: disk, bpm: bpm, tree: tree, activeDataPage: dataID, secIndex: index.NewSecondaryIndex()}
	// A valid meta page first: filling the data page updates it.
	storage.EncodeMeta(metaPage.Data(), storage.MetaData{Magic: storage.MetaMagic, Version: storage.MetaVersion, RootPageID: tree.RootPageID(), ActiveDataPageID: dataID})
	for i := uint64(1); i <= 50; i++ {
		if err := shell.applyPutInMemory(i, []byte(fmt.Sprintf("v1-value-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	big := bytes.Repeat([]byte("L"), 3*MaxRecordSize)
	if err := shell.applyPutInMemory(99, big); err != nil {
		t.Fatal(err)
	}
	storage.EncodeMeta(metaPage.Data(), storage.MetaData{Magic: storage.MetaMagic, RootPageID: tree.RootPageID(), ActiveDataPageID: shell.activeDataPage, LastCheckpointLSN: 10})
	binary.LittleEndian.PutUint32(metaPage.Data()[4:8], 1) // format version 1
	clear(metaPage.Data()[32:64])
	_ = bpm.UnpinPage(metaID, true)
	if err := bpm.FlushAll(); err != nil {
		t.Fatal(err)
	}
	disk.Close()

	w, err := wal.OpenWAL(walPath, true)
	if err != nil {
		t.Fatal(err)
	}
	w.SetCurrentLSN(10)
	var k [8]byte
	binary.LittleEndian.PutUint64(k[:], 7)
	_, _ = w.Write(wal.OpDelete, k[:], nil) // LSN 11
	binary.LittleEndian.PutUint64(k[:], 500)
	_, _ = w.Write(wal.OpPut, k[:], []byte("from-wal-tail")) // LSN 12
	w.Close()
	return dbPath, walPath
}

func TestV1DatabaseMigratesToV2(t *testing.T) {
	dbPath, walPath := createV1Database(t, t.TempDir())

	e, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("open version 1 database: %v", err)
	}
	if !e.disk.Checksums() {
		t.Fatal("migrated database has no page checksums")
	}
	for i := uint64(1); i <= 50; i++ {
		v, ok, err := e.Get(i)
		if i == 7 {
			if ok {
				t.Fatal("delete from the WAL tail was not applied")
			}
			continue
		}
		if err != nil || !ok || string(v) != fmt.Sprintf("v1-value-%d", i) {
			t.Fatalf("key %d: %q ok=%v err=%v", i, v, ok, err)
		}
	}
	if v, _, err := e.Get(99); err != nil || len(v) != 3*MaxRecordSize {
		t.Fatalf("large value: %d bytes, err=%v", len(v), err)
	}
	if v, _, _ := e.Get(500); string(v) != "from-wal-tail" {
		t.Fatalf("put from the WAL tail = %q", v)
	}
	if lsn := e.CurrentLSN(); lsn <= 12 {
		t.Fatalf("LSN after migration = %d; must continue past the old 12", lsn)
	}
	mustPut(t, e, 1000, "after-migration")
	e.Close()

	for _, p := range []string{dbPath + ".v1.bak", walPath + ".v1.bak"} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("old file not kept: %v", err)
		}
	}
	e2, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if v, _, _ := e2.Get(1000); string(v) != "after-migration" {
		t.Fatalf("write after migration lost: %q", v)
	}
}
