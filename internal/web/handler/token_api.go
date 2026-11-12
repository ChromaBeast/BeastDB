package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// GetTokens handles GET /api/tokens — returns all active API tokens.
func (h *APIHandler) GetTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if h.tokens == nil {
		writeJSON(w, map[string]any{"tokens": []any{}})
		return
	}

	tokens := h.tokens.List()
	writeJSON(w, map[string]any{"tokens": tokens, "count": len(tokens)})
}

// CreateToken handles POST /api/tokens — creates a new persistent API token in partition 0x05.
func (h *APIHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	username, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if h.tokens == nil {
		http.Error(w, "Token store unavailable", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10) // 16KB max
	var body struct {
		Name string `json:"name"`
		Role string `json:"role"`
		// ExpiresInSeconds optionally makes the token expire after N seconds (0 = never).
		ExpiresInSeconds int64 `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	tokRole := strings.TrimSpace(body.Role)
	if tokRole == "" {
		tokRole = "admin"
	}

	if !api.ValidTokenRole(tokRole) {
		http.Error(w, "Invalid role: must be admin, writer, viewer or replication", http.StatusBadRequest)
		return
	}
	if body.ExpiresInSeconds < 0 || body.ExpiresInSeconds > 10*365*24*3600 {
		http.Error(w, "Invalid expires_in_seconds", http.StatusBadRequest)
		return
	}
	var expiresAt *time.Time
	if body.ExpiresInSeconds > 0 {
		t := time.Now().Add(time.Duration(body.ExpiresInSeconds) * time.Second)
		expiresAt = &t
	}

	rawToken, meta, err := h.tokens.CreateWithExpiry(name, tokRole, username, expiresAt)
	if err != nil {
		http.Error(w, "Failed to create token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"token": rawToken,
		"meta":  meta,
	})
}

// DeleteToken handles DELETE /api/tokens?id=... — revokes a token from BeastDB.
func (h *APIHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, err := h.sessions.ValidateSession(r)
	if err != nil || role != auth.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "Token id is required", http.StatusBadRequest)
		return
	}

	if h.tokens == nil {
		http.Error(w, "Token store unavailable", http.StatusInternalServerError)
		return
	}

	if err := h.tokens.Delete(id); err != nil {
		http.Error(w, "Failed to revoke token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
