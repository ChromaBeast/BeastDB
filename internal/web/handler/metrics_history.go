package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

const (
	defaultHistoryWindow = time.Hour
	maxHistoryWindow     = 24 * time.Hour
	maxHistoryPoints     = 360
)

// MetricsHistoryHandler serves GET /api/metrics/history?window=<seconds> (admin only).
// The history lives in memory only and starts empty after a restart; the
// response says how far back it can actually reach.
// Body: {"intervalSeconds":N,"windowSeconds":N,"points":[metrics.Point...]}.
func MetricsHistoryHandler(sessions *auth.SessionManager, hist *metrics.History, interval time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, role, err := sessions.ValidateSession(r); err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		window := defaultHistoryWindow
		if v := r.URL.Query().Get("window"); v != "" {
			secs, err := strconv.Atoi(v)
			if err != nil || secs <= 0 {
				http.Error(w, "invalid window", http.StatusBadRequest)
				return
			}
			window = time.Duration(secs) * time.Second
			if window > maxHistoryWindow {
				window = maxHistoryWindow
			}
		}
		points := []metrics.Point{}
		if hist != nil {
			points = hist.Window(time.Now(), window, maxHistoryPoints)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, map[string]any{
			"intervalSeconds": int(interval / time.Second),
			"windowSeconds":   int(window / time.Second),
			"points":          points,
		})
	})
}
