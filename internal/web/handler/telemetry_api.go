package handler

import "net/http"

// GetStats returns live engine telemetry and the configured node role.
func (h *APIHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"lsn":     h.engine.CurrentLSN(),
		"role":    h.role,
		"mode":    "B+ Tree (4KB)",
		"version": h.version,
	})
}
