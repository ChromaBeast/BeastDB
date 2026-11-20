package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/replication"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

type fakeReplicas struct{ list []replication.ReplicaInfo }

func (f fakeReplicas) Replicas() []replication.ReplicaInfo { return f.list }

func opsServer(t *testing.T, src fakeReplicas, withSource bool) (*Server, *api.Engine, []*http.Cookie, []*http.Cookie) {
	t.Helper()
	opts := Options{}
	if withSource {
		opts.Replication = src
	}
	return opsServerWith(t, opts)
}

func opsServerWith(t *testing.T, opts Options) (*Server, *api.Engine, []*http.Cookie, []*http.Cookie) {
	t.Helper()
	dir := t.TempDir()
	engine, err := api.NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	users := auth.NewUserStore(engine)
	if err := users.CreateUser("root", "admin-pass", auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := users.CreateUser("viewer", "viewer-pass", auth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServerWithOptions("127.0.0.1:0", engine, make([]byte, 32), "leader", "test", nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	login := func(user, pass string) []*http.Cookie {
		form := url.Values{"username": {user}, "password": {pass}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.httpServer.Handler.ServeHTTP(rec, req)
		if len(rec.Result().Cookies()) == 0 {
			t.Fatalf("login failed for %s: %d", user, rec.Code)
		}
		return rec.Result().Cookies()
	}
	return srv, engine, login("root", "admin-pass"), login("viewer", "viewer-pass")
}

func do(srv *Server, method, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func TestCheckpointEndpoint(t *testing.T) {
	srv, engine, admin, viewer := opsServer(t, fakeReplicas{}, false)
	if err := engine.Put(1, []byte("v")); err != nil {
		t.Fatal(err)
	}

	if rec := do(srv, http.MethodPost, "/api/checkpoint", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous: want 403, got %d", rec.Code)
	}
	if rec := do(srv, http.MethodPost, "/api/checkpoint", viewer); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: want 403, got %d", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/checkpoint", admin); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: want 405, got %d", rec.Code)
	}

	rec := do(srv, http.MethodPost, "/api/checkpoint", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK                 bool  `json:"ok"`
		LastCheckpointUnix int64 `json:"lastCheckpointUnix"`
		DirtyPages         int   `json:"dirtyPages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.LastCheckpointUnix == 0 || body.DirtyPages != 0 {
		t.Fatalf("checkpoint did not run: %+v", body)
	}
}

func TestReplicationEndpoint(t *testing.T) {
	src := fakeReplicas{list: []replication.ReplicaInfo{
		{ID: "r1", Address: "10.0.0.2:5555", Connected: true, AckedLSN: 1},
		{ID: "r2", AckedLSN: 99},
	}}
	srv, engine, admin, viewer := opsServer(t, src, true)
	for k := uint64(1); k <= 3; k++ {
		if err := engine.Put(k, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if rec := do(srv, http.MethodGet, "/api/replication", viewer); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: want 403, got %d", rec.Code)
	}
	rec := do(srv, http.MethodGet, "/api/replication", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Role     string `json:"role"`
		LSN      uint64 `json:"lsn"`
		Tracked  bool   `json:"tracked"`
		Replicas []struct {
			ID     string `json:"id"`
			LagLSN uint64 `json:"lagLSN"`
		} `json:"replicas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Role != "leader" || !body.Tracked || len(body.Replicas) != 2 || body.LSN == 0 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Replicas[0].LagLSN != body.LSN-1 || body.Replicas[1].LagLSN != 0 {
		t.Fatalf("bad lag: %+v (lsn %d)", body.Replicas, body.LSN)
	}
}

func TestReplicationEndpointUntracked(t *testing.T) {
	srv, _, admin, _ := opsServer(t, fakeReplicas{}, false)
	rec := do(srv, http.MethodGet, "/api/replication", admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tracked":false`) || !strings.Contains(rec.Body.String(), `"replicas":[]`) {
		t.Fatalf("untracked: %d %s", rec.Code, rec.Body.String())
	}
}
