package storage

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage/simfile"
)

type simDisk struct{ data, journal *simfile.File }

func newSimDisk() simDisk {
	return simDisk{data: simfile.New("data"), journal: simfile.New("journal")}
}

func (s simDisk) open(t *testing.T) *DiskManager {
	t.Helper()
	d, err := OpenDiskManagerFiles(s.data, s.journal, simfile.New("spill"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (s simDisk) crash(rng *rand.Rand, keep float64) simDisk {
	return simDisk{data: s.data.Crash(rng, keep), journal: s.journal.Crash(rng, keep)}
}

func filledPage(b byte) []byte { return bytes.Repeat([]byte{b}, PageSize) }

// readAll returns the first byte of every page, which identifies its version.
func readAll(t *testing.T, d *DiskManager, n uint64) []byte {
	t.Helper()
	out := make([]byte, n)
	buf := make([]byte, PageSize)
	for id := uint64(0); id < n; id++ {
		clear(buf)
		if err := d.ReadPage(id, buf); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf, filledPage(buf[0])) {
			t.Fatalf("page %d is torn: mixes versions", id)
		}
		out[id] = buf[0]
	}
	return out
}

// A crash at any point of a checkpoint (journal write, data install, journal
// reset) must leave the data file holding either the whole old image or the
// whole new one after reopen, never a mix.
func TestCheckpointIsAtomicAcrossCrash(t *testing.T) {
	const pages = 12
	for _, target := range []string{"journal", "data"} {
		for failAfter := 0; failAfter <= pages+1; failAfter++ {
			for seed := int64(0); seed < 6; seed++ {
				name := fmt.Sprintf("%s-fail%d-seed%d", target, failAfter, seed)
				t.Run(name, func(t *testing.T) {
					rng := rand.New(rand.NewSource(seed))
					s := newSimDisk()
					d := s.open(t)
					var old []PageImage
					for i := 0; i < pages; i++ {
						id, _ := d.AllocatePage()
						old = append(old, PageImage{ID: id, Data: filledPage('A')})
					}
					if err := d.WriteCheckpoint(old); err != nil {
						t.Fatal(err)
					}

					// New image: half the pages via spill, half passed directly.
					var next []PageImage
					for i := 0; i < pages; i++ {
						if i%2 == 0 {
							if err := d.SpillPage(uint64(i), filledPage('B')); err != nil {
								t.Fatal(err)
							}
						} else {
							next = append(next, PageImage{ID: uint64(i), Data: filledPage('B')})
						}
					}
					f := s.data
					if target == "journal" {
						f = s.journal
					}
					f.FailWritesAfter(failAfter)
					err := d.WriteCheckpoint(next)
					f.FailWritesAfter(-1)

					keep := []float64{0, 0.5, 1}[seed%3]
					r := s.crash(rng, keep).open(t)
					got := readAll(t, r, pages)
					allA, allB := bytes.Repeat([]byte{'A'}, pages), bytes.Repeat([]byte{'B'}, pages)
					switch {
					case bytes.Equal(got, allB):
					case bytes.Equal(got, allA):
						if err == nil && keep == 1 {
							t.Fatalf("checkpoint reported success but the old image came back")
						}
					default:
						t.Fatalf("mixed image after crash: %q (checkpoint err %v)", got, err)
					}
				})
			}
		}
	}
}

// Evicted pages go to the spill file: no fsync, readable until the next
// checkpoint, and never visible in the data file after a crash.
func TestSpilledPagesAreScratchOnly(t *testing.T) {
	s := newSimDisk()
	d := s.open(t)
	for i := 0; i < 4; i++ {
		_, _ = d.AllocatePage()
	}
	if err := d.WriteCheckpoint([]PageImage{{0, filledPage('A')}, {1, filledPage('A')}, {2, filledPage('A')}, {3, filledPage('A')}}); err != nil {
		t.Fatal(err)
	}
	syncs := s.data.Syncs() + s.journal.Syncs()
	for round := 0; round < 50; round++ {
		if err := d.SpillPage(uint64(round%4), filledPage('S')); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.data.Syncs() + s.journal.Syncs(); got != syncs {
		t.Fatalf("spilling pages cost %d fsyncs, want 0", got-syncs)
	}
	if got := readAll(t, d, 4); !bytes.Equal(got, []byte("SSSS")) {
		t.Fatalf("spilled pages read back as %q", got)
	}
	r := s.crash(rand.New(rand.NewSource(1)), 1).open(t)
	if got := readAll(t, r, 4); !bytes.Equal(got, []byte("AAAA")) {
		t.Fatalf("after crash the data file holds %q, want the checkpoint image", got)
	}
}

// A checkpoint that failed after its journal was complete leaves that journal
// behind. The next checkpoint must install it before writing its own, so the
// data file never mixes the two.
func TestFailedCheckpointJournalIsInstalledFirst(t *testing.T) {
	s := newSimDisk()
	d := s.open(t)
	for i := 0; i < 3; i++ {
		_, _ = d.AllocatePage()
	}
	if err := d.WriteCheckpoint([]PageImage{{0, filledPage('A')}, {1, filledPage('A')}, {2, filledPage('A')}}); err != nil {
		t.Fatal(err)
	}
	s.data.FailWritesAfter(1) // journal completes, install stops after one page
	if err := d.WriteCheckpoint([]PageImage{{0, filledPage('B')}, {1, filledPage('B')}, {2, filledPage('B')}}); err == nil {
		t.Fatal("expected the install to fail")
	}
	s.data.FailWritesAfter(-1)
	if err := d.WriteCheckpoint([]PageImage{{2, filledPage('C')}}); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, d, 3); !bytes.Equal(got, []byte("BBC")) {
		t.Fatalf("data file holds %q, want BBC", got)
	}
}
