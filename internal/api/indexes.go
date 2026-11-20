package api

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ChromaBeast/beastdb/internal/index"
)

// Secondary indexes: the definition is one ordinary record (so it is
// WAL-logged, replicated and backed up); the entries are derived, in-memory
// and rebuilt from the records. See docs/adr/0005-secondary-indexes.md.

const (
	// IndexCatalogKey is the reserved record holding the index definitions.
	IndexCatalogKey uint64 = 0xFF<<56 | 2

	maxIndexes   = 32
	maxFieldPath = 128
	maxPathDepth = 8
)

var (
	ErrInvalidIndex         = errors.New("indexes: invalid definition")
	ErrIndexExists          = errors.New("indexes: name already in use")
	ErrIndexNotFound        = errors.New("indexes: not found")
	ErrIndexBuilding        = errors.New("indexes: index is still building")
	ErrTooManyIndexes       = errors.New("indexes: limit of 32 indexes reached")
	ErrCollectionHasIndexes = errors.New("collections: drop the collection's indexes first")
	ErrIndexFilter          = errors.New("indexes: filter does not match the index type")
)

func parseFieldPath(field string) ([][]byte, bool) {
	if len(field) == 0 || len(field) > maxFieldPath {
		return nil, false
	}
	segs := strings.Split(field, ".")
	if len(segs) > maxPathDepth {
		return nil, false
	}
	out := make([][]byte, len(segs))
	for i, s := range segs {
		if s == "" {
			return nil, false
		}
		for j := 0; j < len(s); j++ {
			c := s[j]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return nil, false
			}
		}
		out[i] = []byte(s)
	}
	return out, true
}

func (e *Engine) loadIndexCatalog() ([]IndexDef, []byte, error) {
	raw, ok, err := e.Get(IndexCatalogKey)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, nil
	}
	defs, err := decodeIndexCatalog(raw)
	if err != nil {
		return nil, nil, err
	}
	return defs, raw, nil
}

func (e *Engine) storeIndexCatalog(defs []IndexDef, old []byte) error {
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	if len(defs) == 0 {
		return e.DeleteIf(IndexCatalogKey, Condition{Kind: CondValueEquals, Value: old})
	}
	cond := Condition{Kind: CondAbsent}
	if old != nil {
		cond = Condition{Kind: CondValueEquals, Value: old}
	}
	return e.PutIf(IndexCatalogKey, encodeIndexCatalog(defs), cond)
}

// CreateIndex declares an index on a JSON field of a named collection and
// starts building it in the background. kind is index.KindInt or index.KindString.
func (e *Engine) CreateIndex(name, collection, field string, kind int) (IndexDef, error) {
	if !validCollectionName(name) {
		return IndexDef{}, ErrInvalidCollectionName
	}
	if _, ok := parseFieldPath(field); !ok {
		return IndexDef{}, ErrInvalidIndex
	}
	if kind != index.KindInt && kind != index.KindString {
		return IndexDef{}, ErrInvalidIndex
	}
	for attempt := 0; attempt < casRetries; attempt++ {
		cols, err := e.ListCollections()
		if err != nil {
			return IndexDef{}, err
		}
		var col *Collection
		for i := range cols {
			if cols[i].Name == collection {
				col = &cols[i]
			}
		}
		if col == nil {
			return IndexDef{}, ErrCollectionNotFound
		}
		defs, raw, err := e.loadIndexCatalog()
		if err != nil {
			return IndexDef{}, err
		}
		if len(defs) >= maxIndexes {
			return IndexDef{}, ErrTooManyIndexes
		}
		for _, d := range defs {
			if d.Name == name {
				return IndexDef{}, ErrIndexExists
			}
		}
		def := IndexDef{Name: name, Prefix: col.Prefix, Field: field, Kind: kind, CreatedAt: time.Now().Unix()}
		err = e.storeIndexCatalog(append(defs, def), raw)
		if errors.Is(err, ErrConditionFailed) {
			continue
		}
		if err != nil {
			return IndexDef{}, err
		}
		return def, nil
	}
	return IndexDef{}, ErrConditionFailed
}

// DropIndex removes an index definition and frees its entries.
func (e *Engine) DropIndex(name string) error {
	for attempt := 0; attempt < casRetries; attempt++ {
		defs, raw, err := e.loadIndexCatalog()
		if err != nil {
			return err
		}
		idx := -1
		for i, d := range defs {
			if d.Name == name {
				idx = i
			}
		}
		if idx < 0 {
			return ErrIndexNotFound
		}
		rest := append(defs[:idx:idx], defs[idx+1:]...)
		err = e.storeIndexCatalog(rest, raw)
		if errors.Is(err, ErrConditionFailed) {
			continue
		}
		return err
	}
	return ErrConditionFailed
}

// hasIndexesOn reports whether any index definition targets prefix.
func (e *Engine) hasIndexesOn(prefix uint8) (bool, error) {
	defs, _, err := e.loadIndexCatalog()
	if err != nil {
		return false, err
	}
	for _, d := range defs {
		if d.Prefix == prefix {
			return true, nil
		}
	}
	return false, nil
}
