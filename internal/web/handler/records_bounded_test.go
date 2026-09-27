package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestGetRecordsBounded(t *testing.T) {
	keyP1A := (uint64(1) << 56) | 10
	keyP1B := (uint64(1) << 56) | 20
	keyP2A := (uint64(2) << 56) | 10

	eng := &mockEngine{
		records: []api.RecordItem{
			{Key: keyP1A, KeyText: "p1a", Value: "val1"},
			{Key: keyP1B, KeyText: "p1b", Value: "val2"},
			{Key: keyP2A, KeyText: "p2a", Value: "val3"},
		},
	}

	h := NewAPIHandler(eng, auth.NewSessionManager(make([]byte, 32)), "leader", "test", nil)

	// Query bounded to partition 1: start = 1 << 56, end = (2 << 56) - 1
	startKey := uint64(1) << 56
	endKey := (uint64(2) << 56) - 1

	req := httptest.NewRequest(http.MethodGet, "/api/records?start=72057594037927936&end=144115188075855871&limit=50", nil)
	_ = startKey
	_ = endKey
	w := httptest.NewRecorder()
	h.GetRecords(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res struct {
		Count   int              `json:"count"`
		Records []api.RecordItem `json:"records"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	if res.Count != 2 {
		t.Fatalf("expected 2 records in partition 1, got %d", res.Count)
	}
	if res.Records[0].Key != keyP1A || res.Records[1].Key != keyP1B {
		t.Fatalf("unexpected records returned: %+v", res.Records)
	}
}

func TestSearchRecordsTelemetry(t *testing.T) {
	keyP1 := (uint64(1) << 56) | 10
	keyP2 := (uint64(2) << 56) | 20

	eng := &mockEngine{
		records: []api.RecordItem{
			{Key: keyP1, KeyText: "p1", Value: `{"title": "cyberpunk 2077"}`},
			{Key: keyP2, KeyText: "p2", Value: `{"title": "cyberpunk anime"}`},
		},
	}

	h := NewAPIHandler(eng, auth.NewSessionManager(make([]byte, 32)), "leader", "test", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=cyberpunk&prefix=1", nil)
	w := httptest.NewRecorder()
	h.SearchRecords(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res struct {
		Count        int `json:"count"`
		ScannedCount int `json:"scannedCount"`
		Prefix       int `json:"prefix"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	if res.Count != 1 || res.Prefix != 1 {
		t.Fatalf("unexpected search response: %+v", res)
	}
}

