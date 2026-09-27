package api

import (
	"math"
	"strconv"
	"strings"
)

// RecordItem represents a key-value record returned by scan queries.
type RecordItem struct {
	Key     uint64 `json:"key"`
	KeyText string `json:"keyText"`
	Value   string `json:"value"`
}

// ScanRecords retrieves up to limit records starting from startKey in ascending key order.
func (e *Engine) ScanRecords(startKey uint64, limit int) ([]RecordItem, error) {
	recs, _, _, err := e.ScanRecordsPaginated(startKey, limit)
	return recs, err
}

// ScanRecordsPaginated retrieves up to limit records and determines if a subsequent page exists.
// Returns (records, nextKey, hasMore, error).
func (e *Engine) ScanRecordsPaginated(startKey uint64, limit int) ([]RecordItem, uint64, bool, error) {
	return e.ScanRecordsBounded(startKey, math.MaxUint64, limit)
}

// ScanRecordsBounded retrieves up to limit records in [startKey, endKey].
// Returns (records, nextKey, hasMore, error).
func (e *Engine) ScanRecordsBounded(startKey, endKey uint64, limit int) ([]RecordItem, uint64, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	cursor, err := e.Scan(startKey, endKey)
	if err != nil {
		return nil, 0, false, err
	}
	defer cursor.Close()

	// Fetch up to limit + 1 to detect whether another page is available.
	fetchCap := limit + 1
	items := make([]RecordItem, 0, fetchCap)

	for len(items) < fetchCap {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			return nil, 0, false, err
		}
		if !ok {
			break
		}

		valBytes, err := e.ReadTuple(rid)
		if err != nil {
			return nil, 0, false, err
		}

		items = append(items, RecordItem{
			Key:     key,
			KeyText: strconv.FormatUint(key, 10),
			Value:   string(valBytes),
		})
	}

	if len(items) > limit {
		return items[:limit], items[limit].Key, true, nil
	}
	return items, 0, false, nil
}

// ScanSearchChunk scans up to maxScan records within [startKey, endKey], testing if
// the tuple value contains query (case-insensitive). Stops when limit matches are found
// or maxScan keys are inspected.
// Returns (matches, scannedCount, nextKey, hasMore, error).
func (e *Engine) ScanSearchChunk(startKey, endKey uint64, maxScan, limit int, query string) ([]RecordItem, int, uint64, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if maxScan <= 0 {
		maxScan = 2500
	}
	if maxScan > 10000 {
		maxScan = 10000
	}

	cursor, err := e.Scan(startKey, endKey)
	if err != nil {
		return nil, 0, 0, false, err
	}
	defer cursor.Close()

	qLower := strings.ToLower(query)
	var matches []RecordItem
	scanned := 0
	var nextKey uint64
	hasMore := false

	for scanned < maxScan {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			return nil, scanned, 0, false, err
		}
		if !ok {
			break
		}

		scanned++

		valBytes, err := e.ReadTuple(rid)
		if err != nil {
			return nil, scanned, 0, false, err
		}

		valStr := string(valBytes)
		if strings.Contains(strings.ToLower(valStr), qLower) {
			matches = append(matches, RecordItem{
				Key:     key,
				KeyText: strconv.FormatUint(key, 10),
				Value:   valStr,
			})
			if len(matches) >= limit {
				nextK, _, nextOk, _ := cursor.Next()
				if nextOk {
					hasMore = true
					nextKey = nextK
				}
				break
			}
		}
	}

	if !hasMore && scanned >= maxScan {
		nextK, _, nextOk, _ := cursor.Next()
		if nextOk {
			hasMore = true
			nextKey = nextK
		}
	}

	return matches, scanned, nextKey, hasMore, nil
}



// ScanPartitionCounts iterates through all leaf keys without loading tuple values
// and returns record counts grouped by the 8-bit partition prefix (key >> 56).
func (e *Engine) ScanPartitionCounts() (map[uint8]int, error) {
	cursor, err := e.Scan(0, math.MaxUint64)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()

	counts := make(map[uint8]int)
	for {
		key, _, ok, err := cursor.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		prefix := uint8(key >> 56)
		counts[prefix]++
	}
	return counts, nil
}

