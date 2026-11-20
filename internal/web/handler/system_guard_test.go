package handler

import (
	"errors"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
)

type guardEngine struct {
	precisionEngine
	puts, deletes int
}

func (e *guardEngine) Put(key uint64, value []byte) error { e.puts++; return nil }
func (e *guardEngine) Delete(key uint64) error            { e.deletes++; return nil }
func (e *guardEngine) ScanRecordsBounded(start, end uint64, limit int) ([]api.RecordItem, uint64, bool, error) {
	return []api.RecordItem{{Key: 1}, {Key: uint64(api.SystemPartition)<<56 | 7}, {Key: 9}}, 0, false, nil
}
func (e *guardEngine) ScanPartitionCounts() (map[uint8]int, error) {
	return map[uint8]int{1: 2, api.SystemPartition: 3}, nil
}

func TestSystemGuardHidesReservedPartition(t *testing.T) {
	raw := &guardEngine{}
	g := guardSystemKeys(raw)
	sys := uint64(api.SystemPartition)<<56 | 7

	if _, found, _ := g.Get(sys); found {
		t.Fatalf("system key is readable through the console")
	}
	if err := g.Put(sys, nil); !errors.Is(err, api.ErrSystemKey) || raw.puts != 0 {
		t.Fatalf("Put on a system key: err=%v puts=%d", err, raw.puts)
	}
	if err := g.PutIfAbsent(sys, nil); !errors.Is(err, api.ErrSystemKey) {
		t.Fatalf("PutIfAbsent on a system key: %v", err)
	}
	if err := g.Delete(sys); !errors.Is(err, api.ErrSystemKey) || raw.deletes != 0 {
		t.Fatalf("Delete on a system key: err=%v deletes=%d", err, raw.deletes)
	}
	items, _, _, _ := g.ScanRecordsBounded(0, ^uint64(0), 10)
	if len(items) != 2 || items[0].Key != 1 || items[1].Key != 9 {
		t.Fatalf("scan through the guard = %+v", items)
	}
	if counts, _ := g.ScanPartitionCounts(); len(counts) != 1 {
		t.Fatalf("partition counts expose the system partition: %v", counts)
	}
	if err := g.Put(5, nil); err != nil || raw.puts != 1 {
		t.Fatalf("ordinary Put blocked: err=%v", err)
	}
}
