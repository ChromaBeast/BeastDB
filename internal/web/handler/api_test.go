package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

const largeKey = uint64(18446744073709551615)

type precisionEngine struct {
	written  uint64
	start    uint64
	existing bool
}

func (e *precisionEngine) Get(key uint64) ([]byte, bool, error) { return []byte("value"), true, nil }
func (e *precisionEngine) Put(key uint64, value []byte) error   { e.written = key; return nil }
func (e *precisionEngine) PutIfAbsent(key uint64, value []byte) error {
	if e.existing {
		return api.ErrRecordExists
	}
	return e.Put(key, value)
}
func (e *precisionEngine) Delete(key uint64) error { return nil }
func (e *precisionEngine) CurrentLSN() uint64      { return 1 }
func (e *precisionEngine) ScanRecords(start uint64, limit int) ([]api.RecordItem, error) {
	return nil, nil
}
func (e *precisionEngine) ScanRecordsPaginated(start uint64, limit int) ([]api.RecordItem, uint64, bool, error) {
	e.start = start
	return []api.RecordItem{{Key: largeKey, KeyText: "18446744073709551615", Value: "value"}}, largeKey, true, nil
}
func (e *precisionEngine) ScanPartitionCounts() (map[uint8]int, error) {
	return nil, nil
}

func TestPreciseKeysInRecordPage(t *testing.T) {
	e := &precisionEngine{}
	h := NewAPIHandler(e, auth.NewSessionManager(make([]byte, 32)), "follower", "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/records?start=18446744073709551615", nil)
	w := httptest.NewRecorder()
	h.GetRecords(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Records []struct {
			KeyText string `json:"keyText"`
		} `json:"records"`
		NextKeyText string `json:"nextKeyText"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if e.start != largeKey || body.NextKeyText != "18446744073709551615" || body.Records[0].KeyText != body.NextKeyText {
		t.Fatalf("precision lost: %+v", body)
	}
}

func TestPutAcceptsStringAndLegacyNumberKeys(t *testing.T) {
	for _, body := range []string{`{"key":"18446744073709551615","value":"hello"}`, `{"key":18446744073709551615,"value":"hello"}`} {
		e := &precisionEngine{}
		sessions := auth.NewSessionManager(make([]byte, 32))
		h := NewAPIHandler(e, sessions, "leader", "test", nil)
		cookieResponse := httptest.NewRecorder()
		sessions.IssueSessionCookie(cookieResponse, &auth.User{Username: "admin", Role: auth.RoleAdmin})
		req := httptest.NewRequest(http.MethodPost, "/api/key", strings.NewReader(body))
		req.AddCookie(cookieResponse.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.PutKey(w, req)
		if w.Code != http.StatusCreated || e.written != largeKey {
			t.Fatalf("status %d, written %d", w.Code, e.written)
		}
	}
}

func TestPutRejectsInvalidKey(t *testing.T) {
	e := &precisionEngine{}
	sessions := auth.NewSessionManager(make([]byte, 32))
	h := NewAPIHandler(e, sessions, "leader", "test", nil)
	cookieResponse := httptest.NewRecorder()
	sessions.IssueSessionCookie(cookieResponse, &auth.User{Username: "admin", Role: auth.RoleAdmin})
	req := httptest.NewRequest(http.MethodPost, "/api/key", strings.NewReader(`{"key":"18446744073709551616","value":"hello"}`))
	req.AddCookie(cookieResponse.Result().Cookies()[0])
	w := httptest.NewRecorder()
	h.PutKey(w, req)
	if w.Code != http.StatusBadRequest || e.written != 0 {
		t.Fatalf("status %d, written %d", w.Code, e.written)
	}
}

func TestCreateOnlyConflictAndViewerPermission(t *testing.T) {
	for _, tc := range []struct {
		role     auth.Role
		existing bool
		status   int
	}{
		{auth.RoleAdmin, true, http.StatusConflict},
		{auth.RoleViewer, false, http.StatusForbidden},
	} {
		e := &precisionEngine{existing: tc.existing}
		sessions := auth.NewSessionManager(make([]byte, 32))
		h := NewAPIHandler(e, sessions, "leader", "test", nil)
		cookieResponse := httptest.NewRecorder()
		sessions.IssueSessionCookie(cookieResponse, &auth.User{Username: "user", Role: tc.role})
		req := httptest.NewRequest(http.MethodPost, "/api/key", strings.NewReader(`{"key":"18446744073709551615","value":"new","createOnly":true}`))
		req.AddCookie(cookieResponse.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.PutKey(w, req)
		if w.Code != tc.status || e.written != 0 {
			t.Fatalf("role %s: status %d, written %d", tc.role, w.Code, e.written)
		}
	}
}
