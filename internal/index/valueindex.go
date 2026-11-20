package index

import (
	"sort"
	"strings"
	"sync"
)

// ValueIndex is an in-memory ordered index of (field value, primary key)
// for one field of one collection. It is derived data: it is rebuilt from the
// records and never written to a data page (docs/adr/0006-secondary-indexes.md).
// Entries are kept in a chunked sorted list so inserts move at most one chunk.
type ValueIndex struct {
	kind int

	mu     sync.RWMutex
	chunks [][]entry         // sorted by (value, key); chunks are non-empty
	byKey  map[uint64]keyVal // current entry of every indexed primary key
	n      int
}

type entry struct {
	i   int64
	s   string
	key uint64
}

type keyVal struct {
	i int64
	s string
}

const chunkMax = 512

// NewValueIndex creates an empty index of kind KindInt or KindString.
func NewValueIndex(kind int) *ValueIndex {
	return &ValueIndex{kind: kind, byKey: make(map[uint64]keyVal)}
}

// Len returns the number of indexed records.
func (x *ValueIndex) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.n
}

func (x *ValueIndex) cmp(a, b entry) int {
	if x.kind == KindInt {
		switch {
		case a.i < b.i:
			return -1
		case a.i > b.i:
			return 1
		}
	} else if c := compareStr(a.s, b.s); c != 0 {
		return c
	}
	switch {
	case a.key < b.key:
		return -1
	case a.key > b.key:
		return 1
	}
	return 0
}

func compareStr(a, b string) int { return strings.Compare(a, b) }

// Set indexes key under v, replacing any previous value of key.
func (x *ValueIndex) Set(key uint64, v FieldValue) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nv := keyVal{i: v.Int}
	if x.kind == KindString {
		nv = keyVal{s: string(v.Str)}
	}
	if old, ok := x.byKey[key]; ok {
		if old == nv {
			return
		}
		x.removeLocked(entry{i: old.i, s: old.s, key: key})
	} else {
		x.n++
	}
	x.byKey[key] = nv
	x.insertLocked(entry{i: nv.i, s: nv.s, key: key})
}

// Remove drops key from the index if present and reports whether it was.
func (x *ValueIndex) Remove(key uint64) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	old, ok := x.byKey[key]
	if !ok {
		return false
	}
	delete(x.byKey, key)
	x.n--
	x.removeLocked(entry{i: old.i, s: old.s, key: key})
	return true
}

// find returns the chunk and offset of the first entry >= e.
func (x *ValueIndex) find(e entry) (int, int) {
	c := sort.Search(len(x.chunks), func(i int) bool {
		ch := x.chunks[i]
		return x.cmp(ch[len(ch)-1], e) >= 0
	})
	if c == len(x.chunks) {
		return c, 0
	}
	ch := x.chunks[c]
	o := sort.Search(len(ch), func(i int) bool { return x.cmp(ch[i], e) >= 0 })
	return c, o
}

func (x *ValueIndex) insertLocked(e entry) {
	if len(x.chunks) == 0 {
		x.chunks = [][]entry{{e}}
		return
	}
	c, o := x.find(e)
	if c == len(x.chunks) { // greater than everything: append to the last chunk
		c = len(x.chunks) - 1
		o = len(x.chunks[c])
	}
	ch := append(x.chunks[c], entry{})
	copy(ch[o+1:], ch[o:])
	ch[o] = e
	x.chunks[c] = ch
	if len(ch) > chunkMax {
		half := len(ch) / 2
		right := append([]entry(nil), ch[half:]...)
		x.chunks[c] = ch[:half:half]
		x.chunks = append(x.chunks, nil)
		copy(x.chunks[c+2:], x.chunks[c+1:])
		x.chunks[c+1] = right
	}
}

func (x *ValueIndex) removeLocked(e entry) {
	c, o := x.find(e)
	if c == len(x.chunks) || x.cmp(x.chunks[c][o], e) != 0 {
		return
	}
	ch := x.chunks[c]
	copy(ch[o:], ch[o+1:])
	ch = ch[:len(ch)-1]
	if len(ch) == 0 {
		x.chunks = append(x.chunks[:c], x.chunks[c+1:]...)
		return
	}
	x.chunks[c] = ch
}

// Bound is an optional inclusive range limit.
type Bound struct {
	Set bool
	Int int64
	Str string
}

// Range returns the primary keys whose value lies in [lo, hi] (unset bounds are
// open), ordered by (value, key).
func (x *ValueIndex) Range(lo, hi Bound) []uint64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	var out []uint64
	c, o := 0, 0
	if lo.Set {
		c, o = x.find(entry{i: lo.Int, s: lo.Str})
	}
	for ; c < len(x.chunks); c, o = c+1, 0 {
		ch := x.chunks[c]
		for ; o < len(ch); o++ {
			e := ch[o]
			if hi.Set {
				if x.kind == KindInt && e.i > hi.Int || x.kind == KindString && e.s > hi.Str {
					return out
				}
			}
			out = append(out, e.key)
		}
	}
	return out
}
