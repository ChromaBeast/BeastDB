package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestEmbeddedStudioServesAuthenticatedBundle(t *testing.T) {
	dir := t.TempDir()
	engine, err := api.NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := auth.NewUserStore(engine).CreateUser("operator", "test-password", auth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer("127.0.0.1:0", engine, make([]byte, 32), "follower", "test-version", nil)

	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"username": {"operator"}, "password": {"test-password"}}
	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.httpServer.Handler.ServeHTTP(login, request)
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login status %d", login.Code)
	}
	if len(login.Result().Cookies()) == 0 {
		t.Fatal("login did not issue session")
	}
	page := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(login.Result().Cookies()[0])
	server.httpServer.Handler.ServeHTTP(page, request)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "BeastDB Studio") {
		t.Fatalf("embedded page: %d", page.Code)
	}
	me := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.AddCookie(login.Result().Cookies()[0])
	server.httpServer.Handler.ServeHTTP(me, request)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"role":"viewer"`) {
		t.Fatalf("session endpoint: %d %s", me.Code, me.Body.String())
	}
}

func TestHealthzAndReadyzEndpoints(t *testing.T) {
	dir := t.TempDir()
	engine, err := api.NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	server, err := NewServer("127.0.0.1:0", engine, make([]byte, 32), "leader", "1.0.0-test", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Test unauthenticated /healthz
	healthRec := httptest.NewRecorder()
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.httpServer.Handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("healthz status %d, body: %s", healthRec.Code, healthRec.Body.String())
	}
	if !strings.Contains(healthRec.Body.String(), `"status":"ok"`) || !strings.Contains(healthRec.Body.String(), `"version":"1.0.0-test"`) {
		t.Fatalf("unexpected healthz body: %s", healthRec.Body.String())
	}

	// Test unauthenticated /readyz
	readyRec := httptest.NewRecorder()
	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	server.httpServer.Handler.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("readyz status %d, body: %s", readyRec.Code, readyRec.Body.String())
	}
	if !strings.Contains(readyRec.Body.String(), `"status":"ready"`) || !strings.Contains(readyRec.Body.String(), `"role":"leader"`) {
		t.Fatalf("unexpected readyz body: %s", readyRec.Body.String())
	}
}

