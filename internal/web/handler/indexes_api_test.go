package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestIndexesEndpointAndFilteredRecords(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	sm := auth.NewSessionManager(make([]byte, 32))
	h := NewAPIHandler(eng, sm, "leader", "test", nil)

	do := func(role auth.Role, fn http.HandlerFunc, method, url, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: role})
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.AddCookie(rec.Result().Cookies()[0])
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}
	idx := func(role auth.Role, method, url, body string) *httptest.ResponseRecorder {
		return do(role, h.IndexesHandler, method, url, body)
	}
	if _, err := eng.CreateCollection("users", nil); err != nil {
		t.Fatal(err)
	}
	cols, _ := eng.ListCollections()
	for n := uint64(1); n <= 10; n++ {
		k, _ := api.CollectionKey(cols[0].Prefix, n)
		_ = eng.Put(k, []byte(`{"age":`+string(rune('0'+n%5))+`,"tier":"t`+string(rune('a'+n%2))+`"}`))
	}

	if w := idx(auth.RoleViewer, http.MethodPost, "/api/indexes", `{"name":"by-age","collection":"users","field":"age","type":"int"}`); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create: %d", w.Code)
	}
	if w := idx(auth.RoleAdmin, http.MethodPost, "/api/indexes", `{"name":"by-age","collection":"users","field":"age","type":"float"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad type: %d", w.Code)
	}
	if w := idx(auth.RoleAdmin, http.MethodPost, "/api/indexes", `{"name":"by-age","collection":"ghost","field":"age","type":"int"}`); w.Code != http.StatusNotFound {
		t.Fatalf("ghost collection: %d", w.Code)
	}
	if w := idx(auth.RoleAdmin, http.MethodPost, "/api/indexes", `{"name":"by-age","collection":"users","field":"age","type":"int"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := idx(auth.RoleAdmin, http.MethodPost, "/api/indexes", `{"name":"by-age","collection":"users","field":"age","type":"int"}`); w.Code != http.StatusConflict {
		t.Fatalf("dup: %d", w.Code)
	}
	var list []IndexEntry
	for i := 0; i < 500; i++ {
		w := idx(auth.RoleViewer, http.MethodGet, "/api/indexes", "")
		list = nil
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list) == 1 && list[0].State == "ready" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(list) != 1 || list[0].State != "ready" || list[0].Entries != 10 || list[0].Type != "int" || list[0].Collection != "users" {
		t.Fatalf("list: %+v", list)
	}

	rec := func(q string) (int, map[string]any) {
		w := do(auth.RoleViewer, h.GetRecords, http.MethodGet, "/api/records?"+q, "")
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	if code, m := rec("index=by-age&eq=2"); code != 200 || m["count"].(float64) != 2 {
		t.Fatalf("eq: %d %v", code, m)
	}
	if code, m := rec("index=by-age&gte=1&lte=2&limit=3"); code != 200 || m["count"].(float64) != 3 || m["hasMore"] != true {
		t.Fatalf("range: %d %v", code, m)
	}
	if code, _ := rec("index=by-age&eq=abc"); code != http.StatusBadRequest {
		t.Fatalf("bad value: %d", code)
	}
	if code, _ := rec("index=by-age"); code != http.StatusBadRequest {
		t.Fatalf("no predicate: %d", code)
	}
	if code, _ := rec("index=nope&eq=1"); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}

	if w := idx(auth.RoleViewer, http.MethodDelete, "/api/indexes?name=by-age", ""); w.Code != http.StatusForbidden {
		t.Fatalf("viewer drop: %d", w.Code)
	}
	if w := idx(auth.RoleAdmin, http.MethodDelete, "/api/indexes?name=by-age", ""); w.Code != http.StatusNoContent {
		t.Fatalf("drop: %d", w.Code)
	}
	if w := idx(auth.RoleAdmin, http.MethodDelete, "/api/indexes?name=by-age", ""); w.Code != http.StatusNotFound {
		t.Fatalf("drop again: %d", w.Code)
	}
}
