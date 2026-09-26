package handler

import (
	"net/http"
	"strconv"
	"strings"
)

// SearchRecords handles GET /api/search?q=<string>&prefix=<uint8>&limit=<int>.
// q is required (min 2 chars). prefix filters by partition (top 8 bits of key >> 56);
// 0 means all partitions. limit defaults to 50, capped at 200.
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
	if ps := r.URL.Query().Get("prefix"); ps != "" {
		p, err := strconv.ParseUint(ps, 10, 8)
		if err != nil {
			http.Error(w, "Invalid prefix parameter", http.StatusBadRequest)
			return
		}
		prefix = uint8(p)
	}

	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		l, err := strconv.Atoi(ls)
		if err != nil {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		if l < 1 {
			l = 1
		}
		if l > 200 {
			l = 200
		}
		limit = l
	}

	all, err := h.engine.ScanRecords(0, 500000)
	if err != nil {
		http.Error(w, "Engine scan error", http.StatusInternalServerError)
		return
	}

	type resultItem struct {
		KeyText string `json:"keyText"`
		Value   string `json:"value"`
	}

	qLower := strings.ToLower(q)
	var matched []resultItem
	hasMore := false

	for _, item := range all {
		if prefix > 0 && uint8(item.Key>>56) != prefix {
			continue
		}
		if !strings.Contains(strings.ToLower(item.Value), qLower) {
			continue
		}
		if len(matched) >= limit {
			hasMore = true
			break
		}
		matched = append(matched, resultItem{
			KeyText: strconv.FormatUint(item.Key, 10),
			Value:   item.Value,
		})
	}

	writeJSON(w, map[string]any{
		"records": matched,
		"count":   len(matched),
		"hasMore": hasMore,
	})
}
