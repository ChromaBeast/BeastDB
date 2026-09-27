package api

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// createLegacyV0Database creates a v0 database layout where Page 0 is the B+ tree root.
func createLegacyV0Database(t *testing.T, dbPath string, records map[uint64][]byte) {
	t.Helper()
	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		t.Fatalf("failed to open disk manager: %v", err)
	}
	defer disk.Close()

	bpm := storage.NewBufferPoolManager(disk, 16)

	// In v0, Page 0 was created by CreateBPlusTree.
	rootPage, rootID, err := bpm.NewPage()
	if err != nil || rootID != 0 {
		t.Fatalf("expected root page 0, got %d, err: %v", rootID, err)
	}
	index.InitLeafNode(rootPage.Data(), true, 0)
	leaf := index.AsLeafNode(rootPage.Data())

	// Page 1 was the active data page.
	dataPage, dataID, err := bpm.NewPage()
	if err != nil || dataID != 1 {
		t.Fatalf("expected data page 1, got %d, err: %v", dataID, err)
	}

	for k, v := range records {
		slotID, err := dataPage.InsertTuple(v)
		if err != nil {
			t.Fatalf("failed to insert tuple: %v", err)
		}
		leaf.Insert(k, storage.RID{PageID: dataID, SlotID: slotID})
	}

	_ = bpm.UnpinPage(rootID, true)
	_ = bpm.UnpinPage(dataID, true)
	if err := bpm.FlushAll(); err != nil {
		t.Fatalf("failed to flush pages: %v", err)
	}
}

func TestLegacyV0DatabaseMigration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "beast.bin")
	walPath := filepath.Join(tmpDir, "beast.wal")

	initialRecords := map[uint64][]byte{
		1001: []byte("user-alice"),
		1002: []byte("game-elden-ring"),
		2005: []byte("movie-interstellar"),
	}

	createLegacyV0Database(t, dbPath, initialRecords)

	// Append an extra record to the legacy WAL.
	w, err := wal.OpenWAL(walPath, true)
	if err != nil {
		t.Fatalf("failed to open WAL: %v", err)
	}
	var kBuf [8]byte
	binary.LittleEndian.PutUint64(kBuf[:], 3001)
	_, _ = w.Write(wal.OpPut, kBuf[:], []byte("book-dune"))
	_ = w.Close()

	// NewEngine should detect the legacy v0 database, migrate it to v1, and open successfully!
	engine, err := NewEngine(dbPath, walPath, 32)
	if err != nil {
		t.Fatalf("NewEngine failed on legacy database: %v", err)
	}
	defer engine.Close()

	// Verify all initial records were migrated.
	for k, expected := range initialRecords {
		val, found, err := engine.Get(k)
		if err != nil || !found {
			t.Fatalf("expected key %d to be found, err: %v", k, err)
		}
		if string(val) != string(expected) {
			t.Fatalf("key %d value mismatch: got %q, want %q", k, string(val), string(expected))
		}
	}

	// Verify the record from the legacy WAL was also recovered.
	walVal, found, err := engine.Get(3001)
	if err != nil || !found || string(walVal) != "book-dune" {
		t.Fatalf("expected WAL record 3001 to be recovered, got %q (found: %v, err: %v)", string(walVal), found, err)
	}

	// Verify backup file was created.
	if _, err := os.Stat(dbPath + ".v0.bak"); err != nil {
		t.Fatalf("expected legacy backup %s.v0.bak to exist: %v", dbPath, err)
	}
}

func TestUninitializedBlankFileHandling(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "beast.bin")
	walPath := filepath.Join(tmpDir, "beast.wal")

	// Write 4096 zero bytes (a blank, uninitialized page).
	zeroBlock := make([]byte, 4096)
	if err := os.WriteFile(dbPath, zeroBlock, 0644); err != nil {
		t.Fatalf("failed to write blank file: %v", err)
	}

	engine, err := NewEngine(dbPath, walPath, 32)
	if err != nil {
		t.Fatalf("NewEngine failed on blank 1-page file: %v", err)
	}
	defer engine.Close()

	if err := engine.Put(42, []byte("answer")); err != nil {
		t.Fatalf("failed to put record: %v", err)
	}
	val, found, err := engine.Get(42)
	if err != nil || !found || string(val) != "answer" {
		t.Fatalf("unexpected record: %s (found: %v)", val, found)
	}
}

func TestCorruptFileInvalidMagicRejected(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "beast.bin")
	walPath := filepath.Join(tmpDir, "beast.wal")

	// Write random garbage bytes that do not look like a B+ tree node or MetaPage.
	garbage := make([]byte, 4096)
	garbage[0] = 0xFF // Not a valid NodeType
	garbage[1] = 0xFE
	if err := os.WriteFile(dbPath, garbage, 0644); err != nil {
		t.Fatalf("failed to write garbage file: %v", err)
	}

	_, err := NewEngine(dbPath, walPath, 32)
	if err != storage.ErrInvalidMetaMagic {
		t.Fatalf("expected ErrInvalidMetaMagic on corrupt file, got: %v", err)
	}
}
