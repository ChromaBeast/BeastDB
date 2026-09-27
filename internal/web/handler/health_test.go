package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
)

type mockEngineReader struct {
	EngineReader
	lsn uint64
}

func (m *mockEngineReader) CurrentLSN() uint64 {
	return m.lsn
}

func (m *mockEngineReader) StorageMetrics() api.StorageMetrics {
	return api.StorageMetrics{TotalPages: 10}
}

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h := HealthzHandler("v1.0.0")
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp HealthzResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Status != "ok" || resp.Version != "v1.0.0" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestReadyzHandler(t *testing.T) {
	// Ready case
	mock := &mockEngineReader{lsn: 42}
	hReady := ReadyzHandler(mock, "leader")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	hReady(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp ReadyzResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Status != "ready" || resp.Role != "leader" || resp.LSN != 42 || resp.TotalPages != 10 {
		t.Fatalf("unexpected ready response: %+v", resp)
	}

	// Unready case (nil engine)
	hUnready := ReadyzHandler(nil, "follower")
	recUnready := httptest.NewRecorder()
	hUnready(recUnready, req)

	if recUnready.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recUnready.Code)
	}
}
