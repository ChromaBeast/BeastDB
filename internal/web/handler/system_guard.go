package handler

import (
	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// The token partition must be the engine's reserved system partition; this
// fails to compile if the two constants drift apart.
const _ = uint8(api.SystemPartition-auth.TokenPartitionPrefix) + uint8(auth.TokenPartitionPrefix-api.SystemPartition)

// systemGuard hides the reserved system partition from the console's data
// routes: its keys read as absent, are left out of scans and searches, and
// cannot be written. Console users and API tokens are managed only through
// their own stores, which use the unguarded engine.
type systemGuard struct {
	EngineReader
}

func guardSystemKeys(e EngineReader) EngineReader {
	if e == nil {
		return nil
	}
	return systemGuard{e}
}

func (g systemGuard) Get(key uint64) ([]byte, bool, error) {
	if api.IsSystemKey(key) {
		return nil, false, nil
	}
	return g.EngineReader.Get(key)
}

func (g systemGuard) Put(key uint64, value []byte) error {
	if api.IsSystemKey(key) {
		return api.ErrSystemKey
	}
	return g.EngineReader.Put(key, value)
}

func (g systemGuard) PutIfAbsent(key uint64, value []byte) error {
	if api.IsSystemKey(key) {
		return api.ErrSystemKey
	}
	return g.EngineReader.PutIfAbsent(key, value)
}

func (g systemGuard) Delete(key uint64) error {
	if api.IsSystemKey(key) {
		return api.ErrSystemKey
	}
	return g.EngineReader.Delete(key)
}

func withoutSystem(items []api.RecordItem) []api.RecordItem {
	out := items[:0]
	for _, it := range items {
		if !api.IsSystemKey(it.Key) {
			out = append(out, it)
		}
	}
	return out
}

func (g systemGuard) ScanRecords(startKey uint64, limit int) ([]api.RecordItem, error) {
	items, err := g.EngineReader.ScanRecords(startKey, limit)
	return withoutSystem(items), err
}

func (g systemGuard) ScanRecordsPaginated(startKey uint64, limit int) ([]api.RecordItem, uint64, bool, error) {
	items, next, more, err := g.EngineReader.ScanRecordsPaginated(startKey, limit)
	return withoutSystem(items), next, more, err
}

func (g systemGuard) ScanRecordsBounded(startKey, endKey uint64, limit int) ([]api.RecordItem, uint64, bool, error) {
	items, next, more, err := g.EngineReader.ScanRecordsBounded(startKey, endKey, limit)
	return withoutSystem(items), next, more, err
}

func (g systemGuard) ScanSearchChunk(startKey, endKey uint64, maxScan, limit int, query string) ([]api.RecordItem, int, uint64, bool, error) {
	items, scanned, next, more, err := g.EngineReader.ScanSearchChunk(startKey, endKey, maxScan, limit, query)
	return withoutSystem(items), scanned, next, more, err
}

func (g systemGuard) ScanPartitionCounts() (map[uint8]int, error) {
	counts, err := g.EngineReader.ScanPartitionCounts()
	delete(counts, api.SystemPartition)
	return counts, err
}

func (g systemGuard) Search(query string, limit int, prefix uint8) ([]api.RecordItem, error) {
	items, err := g.EngineReader.Search(query, limit, prefix)
	return withoutSystem(items), err
}
