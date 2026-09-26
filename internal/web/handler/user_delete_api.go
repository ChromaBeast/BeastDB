package handler

import (
	"net/http"
	"strconv"

	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// DeleteUser handles DELETE /api/user?userHash=<uint32>.
// Admin role required. Scans all records and deletes any whose embedded
// user hash (bits 56–28 of the key) matches the given userHash.
func (h *APIHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hs := r.URL.Query().Get("userHash")
	if hs == "" {
		http.Error(w, "userHash parameter is required", http.StatusBadRequest)
		return
	}
	hashVal, err := strconv.ParseUint(hs, 10, 32)
	if err != nil {
		http.Error(w, "Invalid userHash parameter", http.StatusBadRequest)
		return
	}
	targetHash := uint32(hashVal)

	records, scanErr := h.engine.ScanRecords(0, 500000)
	if scanErr != nil {
		http.Error(w, "Engine scan error", http.StatusInternalServerError)
		return
	}

	var deletedKeys []uint64
	for _, item := range records {
		recordUserHash := uint32((item.Key >> 28) & 0x0FFFFFFF)
		if recordUserHash != targetHash {
			continue
		}
		if delErr := h.engine.Delete(item.Key); delErr != nil {
			http.Error(w, "Engine delete error", http.StatusInternalServerError)
			return
		}
		deletedKeys = append(deletedKeys, item.Key)
	}

	if deletedKeys == nil {
		deletedKeys = []uint64{}
	}

	writeJSON(w, map[string]any{
		"deleted": len(deletedKeys),
		"keys":    deletedKeys,
	})
}
