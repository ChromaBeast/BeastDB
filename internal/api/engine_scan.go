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
// If limit <= 0, a default of 50 is applied; maximum limit is capped at 500.
func (e *Engine) ScanRecords(startKey uint64, limit int) ([]RecordItem, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	cursor, err := e.Scan(startKey, math.MaxUint64)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()

	records := make([]RecordItem, 0, limit)
	for len(records) < limit {
		key, rid, ok, err := cursor.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		valBytes, err := e.ReadTuple(rid)
		if err != nil {
			return nil, err
		}

		records = append(records, RecordItem{
			Key:   key,
			Value: string(valBytes),
		})
	}

	return records, nil
}
