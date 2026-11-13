package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/logbuf"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func sessionReq(t *testing.T, sm *auth.SessionManager, role auth.Role, url string) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	sm.IssueSessionCookie(rec, &auth.User{Username: "u", Role: role})
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestSettingsForbiddenAndNoSecrets(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	h := SettingsHandler(sm, RuntimeSettings{
		Role: "leader", Version: "9.9", GRPCAddr: "127.0.0.1:50051", WebAddr: "127.0.0.1:8080",
		DataDir: "/data", PoolSize: 128, WALCheckpointMB: 64, ReplicaID: "r1",
		TLSEnabled: true, APITokenConfigured: true, PartitionConfigPath: "/etc/p.json",
	})

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/settings", nil),
		sessionReq(t, sm, auth.RoleViewer, "/api/settings"),
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, "/api/settings"))
	if w.Code != 200 {
		t.Fatalf("admin got %d", w.Code)
	}
	body := w.Body.String()
	for _, banned := range []string{"password", "secret", "BEGIN", "bearer"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(banned)) {
			t.Fatalf("response mentions %q: %s", banned, body)
		}
	}
	var got struct {
		Node     map[string]any `json:"node"`
		Storage  map[string]any `json:"storage"`
		Security map[string]any `json:"security"`
		Limits   map[string]any `json:"limits"`
		Note     string         `json:"note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Node["role"] != "leader" || got.Node["version"] != "9.9" || got.Storage["data_dir"] != "/data" {
		t.Fatalf("bad fields: %s", body)
	}
	if got.Storage["pool_size_frames"] != float64(128) || got.Storage["wal_checkpoint_mb"] != float64(64) {
		t.Fatalf("bad storage: %s", body)
	}
	if got.Security["tls_enabled"] != true || got.Security["api_token_configured"] != true {
		t.Fatalf("bad security: %s", body)
	}
	if got.Limits["max_value_bytes"] != float64(api.MaxValueSize) {
		t.Fatalf("bad limits: %s", body)
	}
	if got.Note == "" {
		t.Fatal("missing note")
	}

	post := sessionReq(t, sm, auth.RoleAdmin, "/api/settings")
	post.Method = http.MethodPost
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST got %d", w.Code)
	}
}

func TestLogsHandler(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	buf := logbuf.New(50)
	buf.Write([]byte("one\nWarning: two\nthree\n"))
	h := LogsHandler(sm, buf)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, sessionReq(t, sm, auth.RoleViewer, "/api/logs"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("anon expected 403, got %d", w.Code)
	}

	type resp struct {
		Entries []logbuf.Entry `json:"entries"`
		Latest  uint64         `json:"latest"`
	}
	get := func(url string) resp {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, url))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", url, w.Code, w.Body.String())
		}
		var r resp
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := get("/api/logs"); len(r.Entries) != 3 || r.Latest != 3 {
		t.Fatalf("%+v", r)
	}
	if r := get("/api/logs?since=2"); len(r.Entries) != 1 || r.Entries[0].Message != "three" {
		t.Fatalf("%+v", r)
	}
	if r := get("/api/logs?level=warn"); len(r.Entries) != 1 || r.Entries[0].Level != "warn" {
		t.Fatalf("%+v", r)
	}
	if r := get("/api/logs?since=3"); r.Entries == nil || len(r.Entries) != 0 || r.Latest != 3 {
		t.Fatalf("%+v", r)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, "/api/logs?level=bogus"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bogus level got %d", w.Code)
	}
}
