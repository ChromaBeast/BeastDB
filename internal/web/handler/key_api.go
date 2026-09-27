package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// maxPutBodySize caps incoming /api/key payloads to 10MB to prevent memory exhaustion DoS.
const maxPutBodySize = 10 << 20

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

// PutKey handles POST /api/key — inserts or updates a key-value pair. Admin role required.
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

	r.Body = http.MaxBytesReader(w, r.Body, maxPutBodySize)

	var body struct {
		Key        json.RawMessage `json:"key"`
		Value      string          `json:"value"`
		CreateOnly bool            `json:"createOnly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad Request or payload too large", http.StatusBadRequest)
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
