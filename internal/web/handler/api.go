package handler

import (
	"encoding/json"
	"errors"
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

// GetKey handles GET /api/key?k=<uint64> — retrieves a value by key.
func (h *APIHandler) GetKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	key, err := parseKeyParam(r)
	if err != nil {
		http.Error(w, "Invalid key parameter", http.StatusBadRequest)
		return
	}

	val, exists, err := h.engine.Get(key)
	if err != nil {
		http.Error(w, "Engine error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]any{"key": key, "keyText": strconv.FormatUint(key, 10), "value": string(val)})
}

// PutKey handles POST /api/key — inserts or updates a key-value pair.
// Admin role required.
func (h *APIHandler) PutKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var body struct {
		Key        json.RawMessage `json:"key"`
		Value      string          `json:"value"`
		CreateOnly bool            `json:"createOnly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	keyText := string(body.Key)
	if len(keyText) > 1 && keyText[0] == '"' {
		if err := json.Unmarshal(body.Key, &keyText); err != nil {
			http.Error(w, "Invalid key", http.StatusBadRequest)
			return
		}
	}
	key, err := strconv.ParseUint(keyText, 10, 64)
	if err != nil {
		http.Error(w, "Invalid key", http.StatusBadRequest)
		return
	}
	var writeErr error
	if body.CreateOnly {
		writeErr = h.engine.PutIfAbsent(key, []byte(body.Value))
	} else {
		writeErr = h.engine.Put(key, []byte(body.Value))
	}
	if errors.Is(writeErr, api.ErrRecordExists) {
		http.Error(w, "Record already exists", http.StatusConflict)
		return
	}
	if writeErr != nil {
		http.Error(w, "Engine error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// DeleteKey handles DELETE /api/key?k=<uint64>. Admin role required.
func (h *APIHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	key, err := parseKeyParam(r)
	if err != nil {
		http.Error(w, "Invalid key parameter", http.StatusBadRequest)
		return
	}

	if err := h.engine.Delete(key); err != nil {
		http.Error(w, "Engine error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseKeyParam reads the ?k= query parameter as a uint64.
func parseKeyParam(r *http.Request) (uint64, error) {
	return strconv.ParseUint(r.URL.Query().Get("k"), 10, 64)
}

// writeJSON writes v as an indented JSON response.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
