package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestMetricsHistoryHandler(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	hist := metrics.NewHistory(10)
	now := time.Now()
	hist.Add(metrics.Point{Time: now.Add(-2 * time.Hour).Unix(), LSN: 1})
	hist.Add(metrics.Point{Time: now.Add(-time.Minute).Unix(), LSN: 2})
	h := MetricsHistoryHandler(sm, hist, 10*time.Second)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/metrics/history", nil),
		sessionReq(t, sm, auth.RoleViewer, "/api/metrics/history"),
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	}

	type resp struct {
		IntervalSeconds int             `json:"intervalSeconds"`
		WindowSeconds   int             `json:"windowSeconds"`
		Points          []metrics.Point `json:"points"`
	}
	get := func(url string) (int, resp) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, url))
		var r resp
		_ = json.Unmarshal(w.Body.Bytes(), &r)
		return w.Code, r
	}

	code, r := get("/api/metrics/history") // default window: 1 hour
	if code != 200 || len(r.Points) != 1 || r.Points[0].LSN != 2 || r.WindowSeconds != 3600 || r.IntervalSeconds != 10 {
		t.Fatalf("default window: %d %+v", code, r)
	}
	if _, r = get("/api/metrics/history?window=86400"); len(r.Points) != 2 {
		t.Fatalf("24h window should include both points: %+v", r)
	}
	if _, r = get("/api/metrics/history?window=999999999"); r.WindowSeconds != 86400 {
		t.Fatalf("window must be capped at 24h: %d", r.WindowSeconds)
	}
	for _, bad := range []string{"abc", "0", "-5"} {
		if code, _ := get("/api/metrics/history?window=" + bad); code != http.StatusBadRequest {
			t.Fatalf("window=%s: got %d, want 400", bad, code)
		}
	}
}
