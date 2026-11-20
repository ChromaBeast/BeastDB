package api

import (
	"log"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/ChromaBeast/beastdb/internal/index"
)

// Index states reported by ListIndexes.
const (
	IndexBuilding = "building"
	IndexReady    = "ready"
)

const buildChunk = 1024

// liveIndex is the in-memory side of one IndexDef. Registry fields are
// guarded by Engine.mu; the counters are atomic so ListIndexes can read them.
type liveIndex struct {
	def   IndexDef
	path  [][]byte
	vi    *index.ValueIndex
	ready atomic.Bool
	gone  atomic.Bool  // dropped; the builder must stop
	built atomic.Int64 // records examined by the background build

	skipMu  sync.Mutex
	skipped map[uint64]struct{} // records in range that could not be indexed
}

// indexRegistry holds the live indexes. Guarded by Engine.mu.
type indexRegistry struct {
	byName   map[string]*liveIndex
	byPrefix [256][]*liveIndex
	started  bool // builders may run (false while the engine is still opening)
	pending  []*liveIndex
	stop     chan struct{}
	wg       sync.WaitGroup
}

func newIndexRegistry() *indexRegistry {
	return &indexRegistry{byName: map[string]*liveIndex{}, stop: make(chan struct{})}
}

// indexValueLocked maintains both the text index and the field indexes.
func (e *Engine) indexFieldsLocked(key uint64, value []byte) {
	if key == IndexCatalogKey {
		e.reconcileIndexesLocked(value)
		return
	}
	for _, li := range e.idx.byPrefix[key>>56] {
		li.apply(key, value)
	}
}

func (e *Engine) unindexFieldsLocked(key uint64) {
	if key == IndexCatalogKey {
		e.reconcileIndexesLocked(nil)
		return
	}
	for _, li := range e.idx.byPrefix[key>>56] {
		li.remove(key)
	}
}

func (li *liveIndex) apply(key uint64, value []byte) {
	if v, ok := index.ExtractField(value, li.path, li.def.Kind); ok {
		li.vi.Set(key, v)
		li.skipMu.Lock()
		delete(li.skipped, key)
		li.skipMu.Unlock()
		return
	}
	li.vi.Remove(key)
	li.skipMu.Lock()
	li.skipped[key] = struct{}{}
	li.skipMu.Unlock()
}

func (li *liveIndex) remove(key uint64) {
	li.vi.Remove(key)
	li.skipMu.Lock()
	delete(li.skipped, key)
	li.skipMu.Unlock()
}

// reconcileIndexesLocked makes the registry match the catalog record value
// (nil = no record). Called under the engine write lock, or during open.
func (e *Engine) reconcileIndexesLocked(raw []byte) {
	var defs []IndexDef
	if raw != nil {
		var err error
		if defs, err = decodeIndexCatalog(raw); err != nil {
			// Never drop working indexes because of an unreadable record; the
			// catalog API reports the corruption to the operator.
			log.Printf("indexes: catalog record unreadable, keeping current indexes: %v", err)
			return
		}
	}
	reg := e.idx
	want := make(map[string]IndexDef, len(defs))
	for _, d := range defs {
		want[d.Name] = d
	}
	for name, li := range reg.byName {
		if d, ok := want[name]; !ok || d != li.def {
			li.gone.Store(true)
			delete(reg.byName, name)
			list := reg.byPrefix[li.def.Prefix]
			for i, x := range list {
				if x == li {
					reg.byPrefix[li.def.Prefix] = append(list[:i:i], list[i+1:]...)
					break
				}
			}
		}
	}
	for _, d := range defs {
		if _, ok := reg.byName[d.Name]; ok {
			continue
		}
		path, ok := parseFieldPath(d.Field)
		if !ok || (d.Kind != index.KindInt && d.Kind != index.KindString) {
			log.Printf("indexes: ignoring invalid definition %q", d.Name)
			continue
		}
		li := &liveIndex{def: d, path: path, vi: index.NewValueIndex(d.Kind), skipped: map[uint64]struct{}{}}
		reg.byName[d.Name] = li
		reg.byPrefix[d.Prefix] = append(reg.byPrefix[d.Prefix], li)
		if reg.started {
			e.startBuild(li)
		} else {
			reg.pending = append(reg.pending, li)
		}
	}
}

// openIndexes registers the indexes recorded in the catalog before WAL replay
// so replay maintains them; builders start after recovery (startIndexBuilds).
func (e *Engine) openIndexes() error {
	_, raw, err := e.loadIndexCatalog()
	if err != nil {
		log.Printf("indexes: catalog unreadable at open, no indexes loaded: %v", err)
		return nil
	}
	e.reconcileIndexesLocked(raw)
	return nil
}

// startIndexBuilds launches the builders queued while the engine opened.
func (e *Engine) startIndexBuilds() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.idx.started = true
	for _, li := range e.idx.pending {
		if !li.gone.Load() {
			e.startBuild(li)
		}
	}
	e.idx.pending = nil
}

// stopIndexBuilds cancels builders and waits for them. Call before Close
// takes the write lock, because a builder may hold the read lock.
func (e *Engine) stopIndexBuilds() {
	select {
	case <-e.idx.stop:
	default:
		close(e.idx.stop)
	}
	e.idx.wg.Wait()
}

func (e *Engine) startBuild(li *liveIndex) {
	e.idx.wg.Add(1)
	go func() {
		defer e.idx.wg.Done()
		e.buildIndex(li)
	}()
}

// buildIndex scans the collection range in chunks. Each chunk reads and
// inserts under one read lock, so it is atomic with respect to writers; writes
// between chunks maintain the index live and are idempotent per key.
func (e *Engine) buildIndex(li *liveIndex) {
	start, end := CollectionRange(li.def.Prefix)
	for {
		select {
		case <-e.idx.stop:
			return
		default:
		}
		if li.gone.Load() {
			return
		}
		last, n, done, err := e.buildChunk(li, start, end)
		if err != nil {
			log.Printf("indexes: build of %q failed: %v", li.def.Name, err)
			return
		}
		li.built.Add(int64(n))
		if done {
			li.ready.Store(true)
			return
		}
		start = last + 1
	}
}

func (e *Engine) buildChunk(li *liveIndex, start, end uint64) (last uint64, n int, done bool, err error) {
	cur, err := e.Scan(start, end) // holds the read lock until Close
	if err != nil {
		return 0, 0, false, err
	}
	defer cur.Close()
	for n < buildChunk {
		key, rid, ok, err := cur.Next()
		if err != nil {
			return 0, n, false, err
		}
		if !ok {
			return last, n, true, nil
		}
		val, err := e.ReadTuple(rid)
		if err != nil {
			return 0, n, false, err
		}
		li.apply(key, val)
		last = key
		n++
	}
	return last, n, false, nil
}

// IndexInfo is the server-reported status of one index.
type IndexInfo struct {
	Name       string
	Collection string // "" when the prefix is not bound to a name
	Prefix     uint8
	Field      string
	Kind       int
	State      string
	Entries    int
	Skipped    int
	Examined   int64
	CreatedAt  int64
}

// ListIndexes reports every index defined in the catalog with its live state.
// It works on replicas. An index that exists in the catalog but not yet in
// memory is reported as building.
func (e *Engine) ListIndexes() ([]IndexInfo, error) {
	defs, _, err := e.loadIndexCatalog()
	if err != nil {
		return nil, err
	}
	cols, err := e.ListCollections()
	if err != nil {
		return nil, err
	}
	names := map[uint8]string{}
	for _, c := range cols {
		names[c.Prefix] = c.Name
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]IndexInfo, 0, len(defs))
	for _, d := range defs {
		info := IndexInfo{Name: d.Name, Collection: names[d.Prefix], Prefix: d.Prefix, Field: d.Field,
			Kind: d.Kind, State: IndexBuilding, CreatedAt: d.CreatedAt}
		if li := e.idx.byName[d.Name]; li != nil && li.def == d {
			info.Entries = li.vi.Len()
			li.skipMu.Lock()
			info.Skipped = len(li.skipped)
			li.skipMu.Unlock()
			info.Examined = li.built.Load()
			if li.ready.Load() {
				info.State = IndexReady
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
