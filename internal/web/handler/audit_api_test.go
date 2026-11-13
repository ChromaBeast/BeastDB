package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func adminCookieReq(t *testing.T, sm *auth.SessionManager, method, url, body string) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: auth.RoleAdmin})
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestAuditRecordsAdminActionsWithoutValues(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	l, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h := NewAPIHandler(&precisionEngine{}, sm, "leader", "test", nil)
	h.SetAudit(l)

	// A write and a delete by an admin.
	w := httptest.NewRecorder()
	h.PutKey(w, adminCookieReq(t, sm, http.MethodPost, "/api/key", `{"key":"42","value":"top-secret-payload"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("put: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.DeleteKey(w, adminCookieReq(t, sm, http.MethodDelete, "/api/key?k=42", ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}

	entries, _ := l.List(audit.Query{})
	if len(entries) != 2 || entries[0].Action != audit.ActionRecordDelete || entries[1].Action != audit.ActionRecordPut {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[1].Actor != "root" || entries[1].Target != "42" {
		t.Fatalf("actor/target: %+v", entries[1])
	}
	raw, _ := json.Marshal(entries)
	if strings.Contains(string(raw), "top-secret-payload") {
		t.Fatal("record values must never be written to the audit log")
	}

	// A rejected viewer write leaves no entry.
	rec := httptest.NewRecorder()
	sm.IssueSessionCookie(rec, &auth.User{Username: "v", Role: auth.RoleViewer})
	req := httptest.NewRequest(http.MethodPost, "/api/key", strings.NewReader(`{"key":"7","value":"x"}`))
	req.AddCookie(rec.Result().Cookies()[0])
	h.PutKey(httptest.NewRecorder(), req)
	if got, _ := l.List(audit.Query{}); len(got) != 2 {
		t.Fatalf("a forbidden request must not be audited as an action: %+v", got)
	}
}

func TestAuditHandlerAuthAndStatus(t *testing.T) {
	sm := auth.NewSessionManager(make([]byte, 32))
	l, _ := audit.Open(t.TempDir())
	defer l.Close()
	_ = l.Record("root", audit.ActionCheckpoint, "", "LSN 5", "")
	_ = l.Record("root", audit.ActionTokenCreate, "tok_1", "ci · role viewer", "")
	h := AuditHandler(sm, l)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/audit", nil),
		sessionReq(t, sm, auth.RoleViewer, "/api/audit"),
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, "/api/audit?action=token.create"))
	var body struct {
		Entries []audit.Entry `json:"entries"`
		Status  audit.Status  `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("%d %v", w.Code, err)
	}
	if len(body.Entries) != 1 || body.Entries[0].Target != "tok_1" || !body.Status.Verified || body.Status.Entries != 2 {
		t.Fatalf("%+v", body)
	}

	// A nil log (auditing disabled) still answers, with nothing in it.
	w = httptest.NewRecorder()
	AuditHandler(sm, nil).ServeHTTP(w, sessionReq(t, sm, auth.RoleAdmin, "/api/audit"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"entries":[]`) {
		t.Fatalf("nil log: %d %s", w.Code, w.Body.String())
	}
}
