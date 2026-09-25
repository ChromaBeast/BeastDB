package api

import (
	"math"
)

// RecordItem represents a key-value record returned by scan queries.
type RecordItem struct {
	Key   uint64 `json:"key"`
	Value string `json:"value"`
}

// ScanRecords retrieves up to limit records starting from startKey in ascending key order.
func (e *Engine) ScanRecords(startKey uint64, limit int) ([]RecordItem, error) {
	recs, _, _, err := e.ScanRecordsPaginated(startKey, limit)
	return recs, err
}

// ScanRecordsPaginated retrieves up to limit records and determines if a subsequent page exists.
// Returns (records, nextKey, hasMore, error).
func (e *Engine) ScanRecordsPaginated(startKey uint64, limit int) ([]RecordItem, uint64, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	cursor, err := e.Scan(startKey, math.MaxUint64)
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
			Key:   key,
			Value: string(valBytes),
		})
	}

	if len(items) > limit {
		return items[:limit], items[limit].Key, true, nil
	}
	return items, 0, false, nil
}
