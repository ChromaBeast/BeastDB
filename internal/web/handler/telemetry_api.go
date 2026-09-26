package handler

import (
	"fmt"
	"net/http"
)

// GetStats returns live engine telemetry and the configured node role.
// When ?counts=true is present, it also performs a full scan and returns
// partitionCounts — a map of prefix (top 8 bits of key >> 56) to record count.
func (h *APIHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"lsn":     h.engine.CurrentLSN(),
		"role":    h.role,
		"mode":    "B+ Tree (4KB)",
		"version": h.version,
	}

	if r.URL.Query().Get("counts") == "true" {
		counts, err := h.engine.ScanPartitionCounts()
		if err != nil {
			http.Error(w, "Engine scan error", http.StatusInternalServerError)
			return
		}
		strCounts := make(map[string]int, len(counts))
		for prefix, count := range counts {
			strCounts[fmt.Sprintf("%d", prefix)] = count
		}
		resp["partitionCounts"] = strCounts
	}

	writeJSON(w, resp)
}
