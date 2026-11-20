package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/runtimecfg"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// The real server: the runtime setting reaches the sign-in handler, and the
// half-authenticated state is no session on any protected route.
func TestRequireTwoFactorSettingGatesAdminSignIn(t *testing.T) {
	dir := t.TempDir()
	engine, err := api.NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := auth.NewUserStore(engine).CreateUser("root", "test-password", auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	rt, err := runtimecfg.Load(dir, runtimecfg.Values{WALCheckpointMB: 64, LoginMaxAttempts: 5, LoginWindowSeconds: 30, BackupKeep: 7})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServerWithOptions("127.0.0.1:0", engine, make([]byte, 32), "leader", "test", nil, Options{Runtime: rt})
	if err != nil {
		t.Fatal(err)
	}
	signIn := func() *httptest.ResponseRecorder {
		form := url.Values{"username": {"root"}, "password": {"test-password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(rec, req)
		return rec
	}
	has := func(rec *httptest.ResponseRecorder, name string) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		return nil
	}

	if rec := signIn(); has(rec, "beastdb_session") == nil {
		t.Fatalf("default must stay password-only: %d", rec.Code)
	}
	if _, err := rt.Update(map[string]int{"require_2fa_admins": 1}); err != nil {
		t.Fatal(err)
	}
	rec := signIn()
	pending := has(rec, "beastdb_pending")
	if has(rec, "beastdb_session") != nil || pending == nil {
		t.Fatalf("policy on: want pending only, got %d %v", rec.Code, rec.Result().Cookies())
	}
	for _, path := range []string{"/api/me", "/api/stats", "/api/records", "/api/audit", "/api/settings/runtime", "/api/tokens", "/api/account/2fa", "/api/backups"} {
		for _, name := range []string{"beastdb_pending", "beastdb_session"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(&http.Cookie{Name: name, Value: pending.Value})
			out := httptest.NewRecorder()
			server.httpServer.Handler.ServeHTTP(out, req)
			if out.Code != http.StatusUnauthorized && out.Code != http.StatusForbidden {
				t.Fatalf("%s with %s cookie: %d", path, name, out.Code)
			}
		}
	}
}
