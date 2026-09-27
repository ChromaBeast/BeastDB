package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// EngineReader is the minimal Engine interface required by API handlers.
type EngineReader interface {
	Get(key uint64) ([]byte, bool, error)
	Put(key uint64, value []byte) error
	PutIfAbsent(key uint64, value []byte) error
	Delete(key uint64) error
	CurrentLSN() uint64
	ScanRecords(startKey uint64, limit int) ([]api.RecordItem, error)
	ScanRecordsPaginated(startKey uint64, limit int) ([]api.RecordItem, uint64, bool, error)
	ScanPartitionCounts() (map[uint8]int, error)
}

// APIHandler handles data access routes under /api/.
type APIHandler struct {
	engine     EngineReader
	sessions   *auth.SessionManager
	role       string
	version    string
	partitions []PartitionEntry
}

// NewAPIHandler creates an APIHandler with engine, session, and partition registry.
func NewAPIHandler(e EngineReader, s *auth.SessionManager, role, version string, partitions []PartitionEntry) *APIHandler {
	return &APIHandler{engine: e, sessions: s, role: role, version: version, partitions: partitions}
}

// GetRecords handles GET /api/records?start=<uint64>&limit=<int>.
func (h *APIHandler) GetRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	startKey := uint64(0)
	if s := r.URL.Query().Get("start"); s != "" {
		if k, err := strconv.ParseUint(s, 10, 64); err == nil {
			startKey = k
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	records, nextKey, hasMore, err := h.engine.ScanRecordsPaginated(startKey, limit)
	if err != nil {
		http.Error(w, "Engine scan error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"records":     records,
		"count":       len(records),
		"nextKey":     nextKey,
		"nextKeyText": strconv.FormatUint(nextKey, 10),
		"hasMore":     hasMore,
	})
}

// writeJSON writes v as an indented JSON response.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
