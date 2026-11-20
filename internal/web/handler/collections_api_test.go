package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestCollectionsEndpoint(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	sm := auth.NewSessionManager(make([]byte, 32))
	h := NewAPIHandler(eng, sm, "leader", "test", nil)

	do := func(role auth.Role, method, url, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: role})
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.AddCookie(rec.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.CollectionsHandler(w, req)
		return w
	}
	const u = "/api/collections"

	if w := do(auth.RoleViewer, http.MethodPost, u, `{"name":"users"}`); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, u, `{"name":"users"}`); w.Code != http.StatusCreated {
		t.Fatalf("admin create: %d %s", w.Code, w.Body.String())
	}
	if w := do(auth.RoleAdmin, http.MethodPost, u, `{"name":"users"}`); w.Code != http.StatusConflict {
		t.Fatalf("dup: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, u, `{"name":"Bad Name"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad name: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, u, `{"name":"t","prefix":5}`); w.Code != http.StatusBadRequest {
		t.Fatalf("reserved: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, u, `{"name":"t","prefix":999}`); w.Code != http.StatusBadRequest {
		t.Fatalf("range: %d", w.Code)
	}
	w := do(auth.RoleViewer, http.MethodGet, u, "")
	var list []CollectionEntry
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].Name != "users" || list[0].Prefix != 0x10 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	k, _ := api.CollectionKey(0x10, 1)
	_ = eng.Put(k, []byte("x"))
	if w := do(auth.RoleAdmin, http.MethodDelete, u+"?name=users", ""); w.Code != http.StatusConflict {
		t.Fatalf("drop non-empty: %d", w.Code)
	}
	_ = eng.Delete(k)
	if w := do(auth.RoleViewer, http.MethodDelete, u+"?name=users", ""); w.Code != http.StatusForbidden {
		t.Fatalf("viewer drop: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodDelete, u+"?name=users", ""); w.Code != http.StatusNoContent {
		t.Fatalf("drop: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodDelete, u+"?name=users", ""); w.Code != http.StatusNotFound {
		t.Fatalf("drop missing: %d", w.Code)
	}
}

func TestCollectionsUnsupportedEngineAndCatalogKeyGuard(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	h := NewAPIHandler(&precisionEngine{}, sm, "leader", "test", nil)
	w := httptest.NewRecorder()
	h.CollectionsHandler(w, httptest.NewRequest(http.MethodGet, "/api/collections", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("got %d", w.Code)
	}

	rec := httptest.NewRecorder()
	sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: auth.RoleAdmin})
	body := `{"key":"` + "18374686479671623681" + `","value":"x"}` // 0xFF00000000000001
	req := httptest.NewRequest(http.MethodPost, "/api/key", strings.NewReader(body))
	req.AddCookie(rec.Result().Cookies()[0])
	w = httptest.NewRecorder()
	h.PutKey(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("catalog key put: %d", w.Code)
	}
}
