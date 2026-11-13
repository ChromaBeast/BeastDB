package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/logbuf"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// LogsHandler serves GET /api/logs?since=<seq>&limit=&level=&source=&q= (admin only).
// The response is {"entries":[{seq,time,level,source,message}],"latest":<seq>}.
func LogsHandler(sessions *auth.SessionManager, buf *logbuf.Buffer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, role, err := sessions.ValidateSession(r); err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if buf == nil {
			writeJSON(w, map[string]any{"entries": []logbuf.Entry{}, "latest": 0})
			return
		}
		qs := r.URL.Query()
		q := logbuf.Query{Text: strings.TrimSpace(qs.Get("q"))}
		if v, err := strconv.ParseUint(qs.Get("since"), 10, 64); err == nil {
			q.Since = v
		}
		if v, err := strconv.Atoi(qs.Get("limit")); err == nil {
			q.Limit = v
		}
		switch lv := strings.ToLower(qs.Get("level")); lv {
		case "debug", "info", "warn", "error":
			q.Level = lv
		case "":
		default:
			http.Error(w, "invalid level", http.StatusBadRequest)
			return
		}
		if src := strings.ToLower(qs.Get("source")); src != "" {
			valid := false
			for _, k := range logbuf.Sources {
				valid = valid || k == src
			}
			if !valid {
				http.Error(w, "invalid source", http.StatusBadRequest)
				return
			}
			q.Source = src
		}
		res := buf.Snapshot(q)
		if res.Entries == nil {
			res.Entries = []logbuf.Entry{}
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, map[string]any{"entries": res.Entries, "latest": res.Latest})
	})
}
