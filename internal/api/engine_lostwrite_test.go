package api

import (
	"bytes"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/storage/simfile"
)

// simDisk is a data file and its journal, both simulated so a test can crash them.
type simDisk struct{ data, journal *simfile.File }

func newSimDisk(dir string) simDisk {
	db := filepath.Join(dir, "sim.bin")
	return simDisk{data: simfile.New(db), journal: simfile.New(storage.JournalPath(db))}
}

func (s simDisk) crash(rng *rand.Rand, keep float64) simDisk {
	return simDisk{data: s.data.Crash(rng, keep), journal: s.journal.Crash(rng, keep)}
}

// openEngineOnSim opens an engine on the simulated files with a fresh spill file.
func openEngineOnSim(t *testing.T, s simDisk, walPath string, poolSize int) *Engine {
	t.Helper()
	disk, err := storage.OpenDiskManagerFiles(s.data, s.journal, simfile.New("spill"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := openEngine(disk, s.data.Name(), walPath, poolSize)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	return e
}

// crashEngine abandons e without flushing its buffer pool, as a power cut would.
// Every acknowledged write is already durable in the WAL (writes return only
// after the group fsync), so closing the WAL handle loses nothing extra.
func crashEngine(e *Engine) { _ = e.wal.Close() }

func lostWriteValue(rng *rand.Rand, tag string) []byte {
	n := 8 + rng.Intn(64)
	if rng.Intn(10) == 0 {
		n = 2000 + rng.Intn(9000) // some values span several pages
	}
	v := bytes.Repeat([]byte{byte('a' + rng.Intn(26))}, n)
	copy(v, tag)
	return v
}

// A crash can lose any write that was not fsynced, in the data file, the
// journal or the spill file, and the file sizes may or may not have persisted.
// With a tiny pool, pages are evicted constantly. Recovery (checkpoint image +
// WAL replay) must still rebuild exactly the committed state.
func TestCrashLosingUnsyncedPageWritesRecovers(t *testing.T) {
	seeds := int64(24)
	if testing.Short() {
		seeds = 6
	}
	for seed := int64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			walPath := filepath.Join(t.TempDir(), "lost.wal")
			s := newSimDisk(t.TempDir())
			e := openEngineOnSim(t, s, walPath, 8) // tiny pool: constant eviction
			oracle := map[uint64][]byte{}

			for round := 0; round < 4; round++ {
				for op := 0; op < 80; op++ {
					key := uint64(rng.Intn(100) + 1)
					tag := fmt.Sprintf("s%d-r%d-o%d", seed, round, op)
					switch rng.Intn(10) {
					case 0, 1:
						if err := e.Delete(key); err != nil {
							t.Fatal(err)
						}
						delete(oracle, key)
					case 2:
						ops := []BatchOperation{
							{Type: BatchOpPut, Key: key, Value: lostWriteValue(rng, tag+"a")},
							{Type: BatchOpPut, Key: uint64(rng.Intn(100) + 1), Value: lostWriteValue(rng, tag+"b")},
							{Type: BatchOpDelete, Key: uint64(rng.Intn(100) + 1)},
						}
						if err := e.BatchWrite(ops); err != nil {
							t.Fatal(err)
						}
						oracle[ops[0].Key] = ops[0].Value
						oracle[ops[1].Key] = ops[1].Value
						delete(oracle, ops[2].Key)
					case 3:
						if rng.Intn(4) == 0 {
							if err := e.Checkpoint(); err != nil {
								t.Fatal(err)
							}
						}
					default:
						v := lostWriteValue(rng, tag)
						if err := e.Put(key, v); err != nil {
							t.Fatal(err)
						}
						oracle[key] = v
					}
				}

				crashEngine(e)
				keep := []float64{0, 0.3, 0.7, 1}[rng.Intn(4)]
				s = s.crash(rng, keep)
				e = openEngineOnSim(t, s, walPath, 8)
				verifyAgainstOracle(t, e, oracle)
			}
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A checkpoint that fails part-way (while writing its journal, or while
// installing pages into the data file) and is followed by a crash must recover
// every committed write. When the engine keeps running instead, the next
// checkpoint must install the leftover journal before its own.
func TestCheckpointCrashMidFlushKeepsCommittedData(t *testing.T) {
	for _, target := range []string{"journal", "data"} {
		for failAfter := 0; failAfter < 10; failAfter++ {
			for _, keepRunning := range []bool{false, true} {
				name := fmt.Sprintf("%s-fail%d-continue%v", target, failAfter, keepRunning)
				t.Run(name, func(t *testing.T) {
					rng := rand.New(rand.NewSource(int64(failAfter)))
					walPath := filepath.Join(t.TempDir(), "ckpt.wal")
					s := newSimDisk(t.TempDir())
					e := openEngineOnSim(t, s, walPath, 16)
					oracle := map[uint64][]byte{}
					put := func(k uint64, tag string) {
						v := lostWriteValue(rng, tag)
						if err := e.Put(k, v); err != nil {
							t.Fatal(err)
						}
						oracle[k] = v
					}
					for k := uint64(1); k <= 100; k++ {
						put(k, fmt.Sprintf("k%d", k))
					}

					f := s.data
					if target == "journal" {
						f = s.journal
					}
					f.FailWritesAfter(failAfter)
					ckErr := e.Checkpoint()
					f.FailWritesAfter(-1)
					if ckErr == nil && target == "data" {
						t.Fatalf("checkpoint succeeded despite a data write failure after %d writes", failAfter)
					}

					if keepRunning {
						for k := uint64(1); k <= 100; k += 3 {
							put(k, fmt.Sprintf("after-%d", k))
						}
						if err := e.Checkpoint(); err != nil {
							t.Fatalf("checkpoint after a failed one: %v", err)
						}
						for k := uint64(2); k <= 100; k += 5 {
							put(k, fmt.Sprintf("tail-%d", k))
						}
					}

					crashEngine(e)
					s = s.crash(rng, []float64{0, 0.5, 1}[failAfter%3])
					e = openEngineOnSim(t, s, walPath, 16)
					verifyAgainstOracle(t, e, oracle)
					if err := e.Close(); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
