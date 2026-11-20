package api

import (
	"sort"

	"github.com/ChromaBeast/beastdb/internal/index"
)

// IndexValue is a typed value in an IndexFilter.
type IndexValue struct {
	IsString bool
	Int      int64
	Str      string
}

// IndexFilter selects records through a secondary index: Eq, or Gte and/or
// Lte (inclusive). At least one bound is required and Eq excludes the others.
type IndexFilter struct {
	Index string
	Eq    *IndexValue
	Gte   *IndexValue
	Lte   *IndexValue
}

func (f IndexFilter) bounds(kind int) (lo, hi index.Bound, err error) {
	conv := func(v *IndexValue) (index.Bound, error) {
		if v == nil {
			return index.Bound{}, nil
		}
		if v.IsString != (kind == index.KindString) {
			return index.Bound{}, ErrIndexFilter
		}
		return index.Bound{Set: true, Int: v.Int, Str: v.Str}, nil
	}
	if f.Eq != nil {
		if f.Gte != nil || f.Lte != nil {
			return lo, hi, ErrIndexFilter
		}
		if lo, err = conv(f.Eq); err != nil {
			return
		}
		return lo, lo, nil
	}
	if f.Gte == nil && f.Lte == nil {
		return lo, hi, ErrIndexFilter
	}
	if lo, err = conv(f.Gte); err != nil {
		return
	}
	hi, err = conv(f.Lte)
	return
}

// IndexScan walks the records matching an IndexFilter in ascending key order.
// The matching key set is resolved once, under the read lock; each Next call
// then re-reads the records under the read lock and drops any record that no
// longer matches, so every returned record matches the filter when it is read.
// Records that start matching after the key set was taken are not returned;
// consecutive chunks are not one consistent snapshot (same as ScanChunk).
type IndexScan struct {
	e      *Engine
	name   string
	lo, hi index.Bound
	keys   []uint64
	pos    int
}

// NewIndexScan resolves filter against its index. It fails with
// ErrIndexNotFound, ErrIndexBuilding or ErrIndexFilter without scanning.
func (e *Engine) NewIndexScan(filter IndexFilter, startKey, endKey uint64) (*IndexScan, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	li := e.idx.byName[filter.Index]
	if li == nil {
		return nil, ErrIndexNotFound
	}
	if !li.ready.Load() {
		return nil, ErrIndexBuilding
	}
	lo, hi, err := filter.bounds(li.def.Kind)
	if err != nil {
		return nil, err
	}
	all := li.vi.Range(lo, hi)
	keys := all[:0]
	for _, k := range all {
		if k >= startKey && k <= endKey {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return &IndexScan{e: e, name: filter.Index, lo: lo, hi: hi, keys: keys}, nil
}

// Remaining is the number of candidate keys not yet read.
func (s *IndexScan) Remaining() int { return len(s.keys) - s.pos }

// Next reads up to maxItems records (stopping early once their values reach
// maxBytes) and reports whether candidates remain.
func (s *IndexScan) Next(maxItems, maxBytes int) (items []KeyValue, more bool, err error) {
	s.e.mu.RLock()
	defer s.e.mu.RUnlock()
	li := s.e.idx.byName[s.name]
	if li == nil {
		return nil, false, ErrIndexNotFound
	}
	size := 0
	for s.pos < len(s.keys) && len(items) < maxItems && (len(items) == 0 || size < maxBytes) {
		k := s.keys[s.pos]
		s.pos++
		rid, err := s.e.tree.Find(k)
		if err == index.ErrKeyNotFound {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		val, err := s.e.readValue(rid)
		if err != nil {
			return nil, false, err
		}
		if v, ok := index.ExtractField(val, li.path, li.def.Kind); !ok || !inBounds(li.def.Kind, v, s.lo, s.hi) {
			continue // changed since the key set was taken
		}
		items = append(items, KeyValue{Key: k, Value: val})
		size += len(val)
	}
	return items, s.pos < len(s.keys), nil
}

func inBounds(kind int, v index.FieldValue, lo, hi index.Bound) bool {
	if kind == index.KindInt {
		return (!lo.Set || v.Int >= lo.Int) && (!hi.Set || v.Int <= hi.Int)
	}
	return (!lo.Set || string(v.Str) >= lo.Str) && (!hi.Set || string(v.Str) <= hi.Str)
}

// ScanIndexed returns up to limit matching records in [startKey, endKey] in
// ascending key order (limit 0 = all). more is true when matches remain after
// the last record returned; resume with that key + 1 as startKey.
func (e *Engine) ScanIndexed(filter IndexFilter, startKey, endKey uint64, limit int) (items []KeyValue, more bool, err error) {
	s, err := e.NewIndexScan(filter, startKey, endKey)
	if err != nil {
		return nil, false, err
	}
	max := limit
	if limit <= 0 {
		max = len(s.keys)
	}
	items, more, err = s.Next(max, int(^uint(0)>>1))
	return items, more, err
}
