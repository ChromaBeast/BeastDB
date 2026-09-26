package handler

import "net/http"

// PartitionEntry describes a logical key-space partition for the Studio UI.
// Projects configure these via -partition-config flag or a partitions.json sidecar.
type PartitionEntry struct {
	Prefix      uint8  `json:"prefix"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// GetPartitions handles GET /api/partitions.
// Returns the server-configured partition registry so the Studio can label
// key prefixes dynamically at runtime — without a Studio rebuild.
func (h *APIHandler) GetPartitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	entries := h.partitions
	if entries == nil {
		entries = []PartitionEntry{}
	}
	writeJSON(w, entries)
}
