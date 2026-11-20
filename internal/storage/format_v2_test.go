package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func checkpointPages(t *testing.T, d *DiskManager, ids ...uint64) {
	t.Helper()
	var imgs []PageImage
	for _, id := range ids {
		p := NewSlottedPage(id)
		if _, err := p.InsertTuple([]byte("payload-for-page")); err != nil {
			t.Fatal(err)
		}
		imgs = append(imgs, PageImage{ID: id, Data: p.Data()})
	}
	if err := d.WriteCheckpoint(imgs); err != nil {
		t.Fatal(err)
	}
}

func TestPageChecksumDetectsDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.bin")
	d, err := OpenDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_, _ = d.AllocatePage()
	}
	checkpointPages(t, d, 0, 1, 2, 3)
	d.Close()

	raw, _ := os.ReadFile(path)
	raw[2*PageSize+100] ^= 0x01                                  // flip a bit in page 2
	copy(raw[3*PageSize:4*PageSize], raw[1*PageSize:2*PageSize]) // page 1's image written at page 3
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	d, err = OpenDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !d.Checksums() {
		t.Fatal("a version 2 file must open with checksums")
	}
	buf := make([]byte, PageSize)
	if err := d.ReadPage(1, buf); err != nil {
		t.Fatalf("intact page: %v", err)
	}
	for _, id := range []uint64{2, 3} {
		if err := d.ReadPage(id, buf); !errors.Is(err, ErrPageChecksum) {
			t.Fatalf("page %d: got %v, want ErrPageChecksum", id, err)
		}
	}
	if err := d.ReadPage(50, buf); err != nil {
		t.Fatalf("a page past the end reads as zeros: %v", err)
	}
}

// Damage to page 0 must not switch a version 2 file into the unchecked mode.
func TestChecksumModeSurvivesDamagedPageZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.bin")
	d, _ := OpenDiskManager(path)
	for i := 0; i < 3; i++ {
		_, _ = d.AllocatePage()
	}
	checkpointPages(t, d, 0, 1, 2)
	d.Close()
	raw, _ := os.ReadFile(path)
	copy(raw[:64], bytes.Repeat([]byte{0xEE}, 64))
	_ = os.WriteFile(path, raw, 0600)

	d, _ = OpenDiskManager(path)
	defer d.Close()
	if !d.Checksums() {
		t.Fatal("checksums turned off because page 0 is damaged")
	}
	if err := d.ReadPage(0, make([]byte, PageSize)); !errors.Is(err, ErrPageChecksum) {
		t.Fatalf("damaged page 0: got %v, want ErrPageChecksum", err)
	}
}

func TestFreeListReusesPages(t *testing.T) {
	d, _ := OpenDiskManager(filepath.Join(t.TempDir(), "f.bin"))
	defer d.Close()
	bpm := NewBufferPoolManager(d, 8)
	var ids []uint64
	for i := 0; i < 5; i++ {
		_, id, err := bpm.NewPage()
		if err != nil {
			t.Fatal(err)
		}
		_ = bpm.UnpinPage(id, true)
		ids = append(ids, id)
	}
	if err := bpm.FreePage(ids[2]); err != nil {
		t.Fatal(err)
	}
	if err := bpm.FreePage(ids[2]); !errors.Is(err, ErrDoubleFree) {
		t.Fatalf("double free: got %v", err)
	}
	if err := bpm.FreePage(MetaPageID); !errors.Is(err, ErrFreeMetaPage) {
		t.Fatalf("free meta page: got %v", err)
	}
	pinned, pid, _ := bpm.NewPage()
	if pid != ids[2] {
		t.Fatalf("NewPage = %d, want reused %d", pid, ids[2])
	}
	if pinned.Header().PageID != pid || pinned.FreeSpace() != PageDataEnd-PageHeaderSize {
		t.Fatalf("reused page not reset: %+v", pinned.Header())
	}
	if err := bpm.FreePage(pid); !errors.Is(err, ErrPageInUse) {
		t.Fatalf("freeing a pinned page: got %v", err)
	}
	_ = bpm.UnpinPage(pid, true)
	if n := d.NumPages(); n != 5 {
		t.Fatalf("file grew to %d pages, want 5", n)
	}
}

// A free list whose head is not actually a free page is abandoned, never followed.
func TestDamagedFreeListIsAbandoned(t *testing.T) {
	d, _ := OpenDiskManager(filepath.Join(t.TempDir(), "f.bin"))
	defer d.Close()
	bpm := NewBufferPoolManager(d, 8)
	_, a, _ := bpm.NewPage()
	_ = bpm.UnpinPage(a, true)
	_, b, _ := bpm.NewPage()
	_ = bpm.UnpinPage(b, true)
	bpm.SetFreeListHead(b) // b is in use, not free
	_, id, err := bpm.NewPage()
	if err != nil || id == b {
		t.Fatalf("NewPage handed out an in-use page: id=%d err=%v", id, err)
	}
	if bpm.FreeListHead() != 0 {
		t.Fatal("damaged free list was not abandoned")
	}
}

func TestChainRoundTrip(t *testing.T) {
	d, _ := OpenDiskManager(filepath.Join(t.TempDir(), "ch.bin"))
	defer d.Close()
	bpm := NewBufferPoolManager(d, 4) // smaller than the chain: forces eviction
	_, meta, _ := bpm.NewPage()       // page 0 is the meta page; a chain never starts there
	_ = bpm.UnpinPage(meta, true)
	data := bytes.Repeat([]byte("0123456789abcdef"), 2000)
	head, pages, err := WriteChain(bpm, 7, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 8 {
		t.Fatalf("expected a multi-page chain, got %d pages", len(pages))
	}
	got, ids, err := ReadChain(bpm, 7, head)
	if err != nil || !bytes.Equal(got, data) || len(ids) != len(pages) {
		t.Fatalf("round trip: %d bytes, err=%v", len(got), err)
	}
	if _, _, err := ReadChain(bpm, 8, head); !errors.Is(err, ErrBadChain) {
		t.Fatalf("wrong kind: got %v", err)
	}
	FreeChain(bpm, pages)
	_, reused, _ := bpm.NewPage()
	if reused != pages[len(pages)-1] {
		t.Fatalf("freed chain pages not reused")
	}
}

// A slotted page from a version 1 file may fill all 4096 bytes; compacting
// it must keep every tuple.
func TestCompactKeepsFullV1Page(t *testing.T) {
	data := make([]byte, PageSize)
	WritePageHeader(data, PageHeader{PageID: 1, Lower: PageHeaderSize, Upper: PageSize})
	p, _ := WrapPage(data)
	var ids []uint16
	for {
		id, err := p.InsertTuple(bytes.Repeat([]byte{byte(len(ids) + 1)}, 100))
		if err != nil {
			break
		}
		ids = append(ids, id)
	}
	_ = p.DeleteTuple(ids[0])
	p.Compact()
	for _, id := range ids[1:] {
		tup, err := p.GetTuple(id)
		if err != nil || !bytes.Equal(tup, bytes.Repeat([]byte{byte(id + 1)}, 100)) {
			t.Fatalf("tuple %d lost by compaction: %v", id, err)
		}
	}
}
