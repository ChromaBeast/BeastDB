package handler

import (
	"math"
	"net/http"
	"strconv"
)

// SearchRecords handles GET /api/search?q=<string>&prefix=<uint8>&start=<uint64>&limit=<int>&maxScan=<int>.
// q is required (min 2 chars). prefix filters by partition (top 8 bits of key >> 56);
// limit defaults to 50, capped at 200. maxScan defaults to 2500, capped at 10000.
func (h *APIHandler) SearchRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		http.Error(w, "q must be at least 2 characters", http.StatusBadRequest)
		return
	}

	var prefix uint8
	hasPrefix := false
	if ps := r.URL.Query().Get("prefix"); ps != "" {
		p, err := strconv.ParseUint(ps, 10, 8)
		if err != nil {
			http.Error(w, "Invalid prefix parameter", http.StatusBadRequest)
			return
		}
		prefix = uint8(p)
		hasPrefix = true
	}

	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if l, err := strconv.Atoi(ls); err == nil && l > 0 {
			if l > 200 {
				l = 200
			}
			limit = l
		}
	}

	maxScan := 2500
	if ms := r.URL.Query().Get("maxScan"); ms != "" {
		if m, err := strconv.Atoi(ms); err == nil && m > 0 {
			if m > 10000 {
				m = 10000
			}
			maxScan = m
		}
	}

	startKey := uint64(0)
	endKey := uint64(math.MaxUint64)

	if hasPrefix {
		startKey = uint64(prefix) << 56
		if prefix < 255 {
			endKey = ((uint64(prefix) + 1) << 56) - 1
		}
	}

	if ss := r.URL.Query().Get("start"); ss != "" {
		if s, err := strconv.ParseUint(ss, 10, 64); err == nil {
			if s >= startKey && s <= endKey {
				startKey = s
			}
		}
	}

	type resultItem struct {
		KeyText string `json:"keyText"`
		Value   string `json:"value"`
	}

	// Fast Path: Query secondary inverted index for instant exact/prefix matches
	if r.URL.Query().Get("start") == "" {
		indexed, err := h.engine.Search(q, limit, prefix)
		if err == nil && len(indexed) > 0 {
			items := make([]resultItem, len(indexed))
			for i, item := range indexed {
				items[i] = resultItem{
					KeyText: item.KeyText,
					Value:   item.Value,
				}
			}
			resp := map[string]any{
				"records":      items,
				"count":        len(items),
				"scannedCount": len(items),
				"hasMore":      false,
				"indexed":      true,
			}
			if hasPrefix {
				resp["prefix"] = prefix
			}
			writeJSON(w, resp)
			return
		}
	}

	matched, scanned, nextKey, hasMore, err := h.engine.ScanSearchChunk(startKey, endKey, maxScan, limit, q)
	if err != nil {
		http.Error(w, "Engine scan error", http.StatusInternalServerError)
		return
	}

	items := make([]resultItem, len(matched))
	for i, item := range matched {
		items[i] = resultItem{
			KeyText: item.KeyText,
			Value:   item.Value,
		}
	}

	resp := map[string]any{
		"records":      items,
		"count":        len(items),
		"scannedCount": scanned,
		"hasMore":      hasMore,
	}
	if hasMore {
		resp["nextKey"] = strconv.FormatUint(nextKey, 10)
	}
	if hasPrefix {
		resp["prefix"] = prefix
	}

	writeJSON(w, resp)
}
