package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/runtimecfg"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestRuntimeSettingsHandler(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	dir := t.TempDir()
	store, _ := runtimecfg.Load(dir, runtimecfg.Values{WALCheckpointMB: 64, LoginMaxAttempts: 5, LoginWindowSeconds: 30, BackupKeep: 7})
	var applied runtimecfg.Values
	store.Subscribe(func(v runtimecfg.Values) { applied = v })
	al, _ := audit.Open(dir)
	defer al.Close()
	h := RuntimeSettingsHandler(sm, store, al)

	do := func(role auth.Role, method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: role})
		req := httptest.NewRequest(method, "/api/settings/runtime", strings.NewReader(body))
		req.AddCookie(rec.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	if w := do(auth.RoleViewer, http.MethodGet, ""); w.Code != http.StatusForbidden {
		t.Fatalf("viewer must be refused: %d", w.Code)
	}
	if w := httptest.NewRecorder(); true {
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/settings/runtime", nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("anonymous must be refused: %d", w.Code)
		}
	}

	w := do(auth.RoleAdmin, http.MethodPatch, `{"wal_checkpoint_mb":256,"login_max_attempts":3}`)
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Values runtimecfg.Values           `json:"values"`
		Ranges map[string]runtimecfg.Range `json:"ranges"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Values.WALCheckpointMB != 256 || resp.Values.LoginMaxAttempts != 3 || resp.Ranges["wal_checkpoint_mb"].Max == 0 {
		t.Fatalf("%+v", resp)
	}
	if applied.WALCheckpointMB != 256 || applied.LoginMaxAttempts != 3 {
		t.Fatalf("change must reach subscribers: %+v", applied)
	}

	entries, _ := al.List(audit.Query{Action: audit.ActionSettingChange})
	if len(entries) != 1 || !strings.Contains(entries[0].Detail, "wal_checkpoint_mb 64 → 256") || !strings.Contains(entries[0].Detail, "login_max_attempts 5 → 3") {
		t.Fatalf("audit: %+v", entries)
	}

	for name, body := range map[string]string{
		"out of range": `{"login_window_seconds":1}`,
		"unknown":      `{"admin_password":5}`,
		"not a number": `{"wal_checkpoint_mb":"big"}`,
		"empty":        `{}`,
	} {
		if w := do(auth.RoleAdmin, http.MethodPatch, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want 400", name, w.Code)
		}
	}
	if got, _ := al.List(audit.Query{Action: audit.ActionSettingChange}); len(got) != 1 {
		t.Fatal("rejected changes must not be audited as changes")
	}
	if store.Get().WALCheckpointMB != 256 {
		t.Fatal("rejected changes must not alter the settings")
	}
}
