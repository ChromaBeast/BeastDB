package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

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
	req := httptest.NewRequest(http.MethodDelete, "/api/user?userHash=19088743", nil)
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
