package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

type mockEngine struct {
	records []api.RecordItem
	deleted []uint64
}

func (m *mockEngine) Get(key uint64) ([]byte, bool, error) { return nil, false, nil }
func (m *mockEngine) Put(key uint64, value []byte) error   { return nil }
func (m *mockEngine) PutIfAbsent(key uint64, value []byte) error { return nil }
func (m *mockEngine) Delete(key uint64) error {
	m.deleted = append(m.deleted, key)
	return nil
}
func (m *mockEngine) CurrentLSN() uint64 { return 42 }
func (m *mockEngine) ScanRecords(startKey uint64, limit int) ([]api.RecordItem, error) {
	return m.records, nil
}
func (m *mockEngine) ScanRecordsPaginated(start uint64, limit int) ([]api.RecordItem, uint64, bool, error) {
	return m.records, 0, false, nil
}

func TestGetPartitions(t *testing.T) {
	parts := []PartitionEntry{
		{Prefix: 3, Label: "Games", Color: "purple"},
		{Prefix: 4, Label: "Movies", Color: "cyan"},
	}
	h := NewAPIHandler(&mockEngine{}, auth.NewSessionManager(make([]byte, 32)), "leader", "test", parts)
	req := httptest.NewRequest(http.MethodGet, "/api/partitions", nil)
	w := httptest.NewRecorder()
	h.GetPartitions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res []PartitionEntry
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Label != "Games" || res[1].Label != "Movies" {
		t.Fatalf("unexpected partitions: %+v", res)
	}
}

func TestStatsWithCounts(t *testing.T) {
	// Key: prefix 3 in top 8 bits: 3 << 56
	key3 := uint64(3) << 56
	key4 := uint64(4) << 56
	eng := &mockEngine{
		records: []api.RecordItem{
			{Key: key3, Value: "game1"},
			{Key: key3 + 1, Value: "game2"},
			{Key: key4, Value: "movie1"},
		},
	}
	h := NewAPIHandler(eng, auth.NewSessionManager(make([]byte, 32)), "leader", "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/stats?counts=true", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	counts, ok := res["partitionCounts"].(map[string]any)
	if !ok {
		t.Fatalf("missing partitionCounts in response: %+v", res)
	}
	if counts["3"] != float64(2) || counts["4"] != float64(1) {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestSearchRecords(t *testing.T) {
	keyA := (uint64(3) << 56) | 100
	keyB := (uint64(4) << 56) | 200
	eng := &mockEngine{
		records: []api.RecordItem{
			{Key: keyA, KeyText: "100", Value: `{"title": "Elden Ring"}`},
			{Key: keyB, KeyText: "200", Value: `{"title": "The Ring Movie"}`},
		},
	}
	h := NewAPIHandler(eng, auth.NewSessionManager(make([]byte, 32)), "leader", "test", nil)

	// Search matching both
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=ring", nil)
	w := httptest.NewRecorder()
	h.SearchRecords(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res struct {
		Count   int `json:"count"`
		Records []struct {
			KeyText string `json:"keyText"`
		} `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("expected 2 matches, got %d", res.Count)
	}

	// Filter by partition 3
	req2 := httptest.NewRequest(http.MethodGet, "/api/search?q=ring&prefix=3", nil)
	w2 := httptest.NewRecorder()
	h.SearchRecords(w2, req2)
	var res2 struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	if res2.Count != 1 {
		t.Fatalf("expected 1 match for prefix 3, got %d", res2.Count)
	}

	// Short query should return 400
	req3 := httptest.NewRequest(http.MethodGet, "/api/search?q=a", nil)
	w3 := httptest.NewRecorder()
	h.SearchRecords(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short query, got %d", w3.Code)
	}
}

func TestDeleteUserCascade(t *testing.T) {
	userHash := uint32(0x1234567)
	// 2 records with userHash in bits 28..55
	key1 := (uint64(1) << 56) | (uint64(userHash) << 28) | 1
	key2 := (uint64(3) << 56) | (uint64(userHash) << 28) | 2
	otherKey := (uint64(3) << 56) | (uint64(0x9999999) << 28) | 3

	eng := &mockEngine{
		records: []api.RecordItem{
			{Key: key1},
			{Key: key2},
			{Key: otherKey},
		},
	}
	sessions := auth.NewSessionManager(make([]byte, 32))
	initRec := httptest.NewRecorder()
	sessions.IssueSessionCookie(initRec, &auth.User{Username: "admin", Role: auth.RoleAdmin})
	cookies := initRec.Result().Cookies()

	h := NewAPIHandler(eng, sessions, "leader", "test", nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/user?userHash=19088743", nil) // 0x1234567 = 19088743
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.DeleteUser(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Deleted int      `json:"deleted"`
		Keys    []uint64 `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 || len(eng.deleted) != 2 {
		t.Fatalf("expected 2 deleted, got %d (engine deleted %d)", res.Deleted, len(eng.deleted))
	}
}
