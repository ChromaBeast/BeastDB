package api

import (
	"math"
	"strconv"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/index"
)

// Search performs secondary index-backed text search over document/value content.
// Instead of scanning hundreds of thousands of records, it looks up matching primary
// keys in the inverted index and fetches only the relevant records from the buffer pool.
func (e *Engine) Search(query string, limit int, prefix uint8) ([]RecordItem, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	tokens := index.TokenizeWords(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	candidateKeys := e.secIndex.LookupIntersect(tokens)
	if len(candidateKeys) == 0 {
		return nil, nil
	}

	terms := strings.Fields(strings.ToLower(query))
	var results []RecordItem

	for _, key := range candidateKeys {
		if prefix > 0 && uint8(key>>56) != prefix {
			continue
		}

		valBytes, found, err := e.Get(key)
		if err != nil || !found {
			continue
		}

		valLower := strings.ToLower(string(valBytes))
		match := true
		for _, term := range terms {
			if !strings.Contains(valLower, term) {
				match = false
				break
			}
		}

		if match {
			results = append(results, RecordItem{
				Key:     key,
				KeyText: strconv.FormatUint(key, 10),
				Value:   string(valBytes),
			})
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// indexValueLocked indexes textual tokens from value for secondary search.
func (e *Engine) indexValueLocked(key uint64, value []byte) {
	if len(value) == 0 {
		e.secIndex.Unindex(key)
		return
	}
	tokens := index.TokenizeWords(string(value))
	e.secIndex.Index(key, tokens)
}

// unindexValueLocked removes secondary index entries for a primary key.
func (e *Engine) unindexValueLocked(key uint64) {
	e.secIndex.Unindex(key)
}

// rebuildSecondaryIndex populates the secondary index from all existing records.
func (e *Engine) rebuildSecondaryIndex() error {
	cursor, err := e.tree.Scan(0, math.MaxUint64)
	if err != nil {
		return err
	}
	defer cursor.Close()

	for {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}

		valBytes, err := e.ReadTuple(rid)
		if err != nil {
			continue
		}
		e.indexValueLocked(key, valBytes)
	}
	return nil
}
