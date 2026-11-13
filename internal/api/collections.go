package api

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Collections are named bindings from a name to an 8-bit key prefix. They
// live in one ordinary record, so they are WAL-logged, replicated and backed
// up like any other data. See docs/adr/0002-collections.md.

const (
	// CatalogKey is the reserved record holding the collection catalog.
	CatalogKey uint64 = 0xFF<<56 | 1

	// DefaultPrefix holds plain small keys; it cannot be bound to a name.
	DefaultPrefix uint8 = 0x00
	// TokenPrefix holds API tokens and cannot be bound or dropped.
	TokenPrefix uint8 = 0x05
	// CatalogPrefix holds the catalog record and cannot be bound.
	CatalogPrefix uint8 = 0xFF

	// firstAutoPrefix..lastAutoPrefix is the range CreateCollection allocates from.
	firstAutoPrefix uint8 = 0x10
	lastAutoPrefix  uint8 = 0xFE

	maxCollectionName = 64
	maxLocalKey       = uint64(1)<<56 - 1
	casRetries        = 8
)

var (
	ErrInvalidCollectionName = errors.New("collections: name must be 1-64 chars of [a-z0-9_-] starting with a letter or digit")
	ErrReservedPrefix        = errors.New("collections: prefix is reserved")
	ErrCollectionExists      = errors.New("collections: name or prefix already bound")
	ErrCollectionNotFound    = errors.New("collections: not found")
	ErrCollectionNotEmpty    = errors.New("collections: collection still holds records")
	ErrNoFreePrefix          = errors.New("collections: no free prefix left")
	ErrCatalogKey            = errors.New("collections: key is reserved for the collection catalog")
)

// CollectionKey composes a key from a prefix and a 56-bit local key.
func CollectionKey(prefix uint8, local uint64) (uint64, error) {
	if local > maxLocalKey {
		return 0, fmt.Errorf("collections: local key %d exceeds 56 bits", local)
	}
	return uint64(prefix)<<56 | local, nil
}

// CollectionRange returns the inclusive key range owned by prefix.
func CollectionRange(prefix uint8) (start, end uint64) {
	return uint64(prefix) << 56, uint64(prefix)<<56 | maxLocalKey
}

// IsCatalogKey reports whether key is the reserved catalog record.
func IsCatalogKey(key uint64) bool { return key == CatalogKey }

func validCollectionName(name string) bool {
	if len(name) == 0 || len(name) > maxCollectionName {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		alnum := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !alnum && (i == 0 || (c != '_' && c != '-')) {
			return false
		}
	}
	return true
}

func bindablePrefix(p uint8) bool {
	return p != DefaultPrefix && p != TokenPrefix && p != CatalogPrefix
}

// loadCatalog returns the current entries and the raw record (nil if absent).
func (e *Engine) loadCatalog() ([]Collection, []byte, error) {
	raw, ok, err := e.Get(CatalogKey)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, nil
	}
	entries, err := decodeCatalog(raw)
	if err != nil {
		return nil, nil, err
	}
	return entries, raw, nil
}

// storeCatalog writes entries only if the record still equals old (nil = absent).
func (e *Engine) storeCatalog(entries []Collection, old []byte) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Prefix < entries[j].Prefix })
	cond := Condition{Kind: CondAbsent}
	if old != nil {
		cond = Condition{Kind: CondValueEquals, Value: old}
	}
	return e.PutIf(CatalogKey, encodeCatalog(entries), cond)
}

// ListCollections returns the catalog ordered by prefix. It works on replicas.
func (e *Engine) ListCollections() ([]Collection, error) {
	entries, _, err := e.loadCatalog()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Prefix < entries[j].Prefix })
	return entries, nil
}

// prefixHasRecords reports whether any key lives under prefix.
func (e *Engine) prefixHasRecords(prefix uint8) (bool, error) {
	start, end := CollectionRange(prefix)
	cur, err := e.Scan(start, end)
	if err != nil {
		return false, err
	}
	defer cur.Close()
	_, _, ok, err := cur.Next()
	return ok, err
}

// CreateCollection binds name to prefix. A nil prefix auto-allocates the
// lowest empty, unbound prefix in 0x10-0xFE. An explicit prefix may already
// hold data (adopting existing records) but never a reserved prefix.
func (e *Engine) CreateCollection(name string, prefix *uint8) (Collection, error) {
	if !validCollectionName(name) {
		return Collection{}, ErrInvalidCollectionName
	}
	if prefix != nil && !bindablePrefix(*prefix) {
		return Collection{}, ErrReservedPrefix
	}
	for attempt := 0; attempt < casRetries; attempt++ {
		entries, raw, err := e.loadCatalog()
		if err != nil {
			return Collection{}, err
		}
		used := [256]bool{}
		for _, c := range entries {
			if c.Name == name {
				return Collection{}, ErrCollectionExists
			}
			used[c.Prefix] = true
		}
		var chosen uint8
		if prefix != nil {
			if used[*prefix] {
				return Collection{}, ErrCollectionExists
			}
			chosen = *prefix
		} else {
			found := false
			for p := int(firstAutoPrefix); p <= int(lastAutoPrefix); p++ {
				if used[p] {
					continue
				}
				has, err := e.prefixHasRecords(uint8(p))
				if err != nil {
					return Collection{}, err
				}
				if !has {
					chosen, found = uint8(p), true
					break
				}
			}
			if !found {
				return Collection{}, ErrNoFreePrefix
			}
		}
		col := Collection{Name: name, Prefix: chosen, CreatedAt: time.Now().Unix()}
		err = e.storeCatalog(append(entries, col), raw)
		if errors.Is(err, ErrConditionFailed) {
			continue // another writer changed the catalog; re-read and retry
		}
		if err != nil {
			return Collection{}, err
		}
		return col, nil
	}
	return Collection{}, ErrConditionFailed
}

// DropCollection unbinds name. It refuses while the prefix holds records.
func (e *Engine) DropCollection(name string) error {
	for attempt := 0; attempt < casRetries; attempt++ {
		entries, raw, err := e.loadCatalog()
		if err != nil {
			return err
		}
		idx := -1
		for i, c := range entries {
			if c.Name == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrCollectionNotFound
		}
		has, err := e.prefixHasRecords(entries[idx].Prefix)
		if err != nil {
			return err
		}
		if has {
			return ErrCollectionNotEmpty
		}
		rest := append(entries[:idx:idx], entries[idx+1:]...)
		if len(rest) == 0 {
			err = e.DeleteIf(CatalogKey, Condition{Kind: CondValueEquals, Value: raw})
		} else {
			err = e.storeCatalog(rest, raw)
		}
		if errors.Is(err, ErrConditionFailed) {
			continue
		}
		return err
	}
	return ErrConditionFailed
}
