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

// ScanIndexed returns up to limit records in [startKey, endKey] that match
// filter, in ascending key order, like Scan. The index lookup and the record
// reads happen under one read lock, so every record returned matches the
// filter at that instant. more is true when another match exists after the
// last returned key; resume by passing that key + 1 as startKey.
func (e *Engine) ScanIndexed(filter IndexFilter, startKey, endKey uint64, limit int) (items []KeyValue, more bool, err error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	li := e.idx.byName[filter.Index]
	if li == nil {
		return nil, false, ErrIndexNotFound
	}
	if !li.ready.Load() {
		return nil, false, ErrIndexBuilding
	}
	lo, hi, err := filter.bounds(li.def.Kind)
	if err != nil {
		return nil, false, err
	}
	all := li.vi.Range(lo, hi)
	keys := all[:0]
	for _, k := range all {
		if k >= startKey && k <= endKey {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if limit > 0 && len(keys) > limit {
		keys, more = keys[:limit], true
	}
	items = make([]KeyValue, 0, len(keys))
	for _, k := range keys {
		rid, err := e.tree.Find(k)
		if err == index.ErrKeyNotFound {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		val, err := e.readValue(rid)
		if err != nil {
			return nil, false, err
		}
		items = append(items, KeyValue{Key: k, Value: val})
	}
	return items, more, nil
}
