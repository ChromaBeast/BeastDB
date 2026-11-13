package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// AuditHandler serves GET /api/audit?before=<seq>&limit=&action=&actor= (admin only).
// Body: {"entries":[...newest first...],"status":{"entries":N,"verified":bool,"error":""}}.
// Verifying re-reads the whole file, so the status is computed per request; the
// log only grows with admin actions, not with data traffic.
func AuditHandler(sessions *auth.SessionManager, l *audit.Log) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, role, err := sessions.ValidateSession(r); err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		qs := r.URL.Query()
		q := audit.Query{Action: strings.TrimSpace(qs.Get("action")), Actor: strings.TrimSpace(qs.Get("actor"))}
		if v, err := strconv.ParseUint(qs.Get("before"), 10, 64); err == nil {
			q.Before = v
		}
		if v, err := strconv.Atoi(qs.Get("limit")); err == nil {
			q.Limit = v
		}
		entries, err := l.List(q)
		if err != nil {
			http.Error(w, "Could not read the audit log", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, map[string]any{"entries": entries, "status": l.Verify()})
	})
}
