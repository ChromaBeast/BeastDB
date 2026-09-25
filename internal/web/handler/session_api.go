package handler

import "net/http"

// GetMe reports the authenticated user's identity and permissions.
func (h *APIHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	username, role, err := h.sessions.ValidateSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"username": username, "role": role})
}
