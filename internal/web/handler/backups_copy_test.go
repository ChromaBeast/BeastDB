package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestBackupsCopyEndpointAndDestinationListing(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	for i := uint64(1); i <= 30; i++ {
		_ = eng.Put(i, []byte("v"))
	}
	sm := auth.NewSessionManager(make([]byte, 32))
	al, _ := audit.Open(dir)
	defer al.Close()
	mgr := backup.NewManager(eng, filepath.Join(dir, "backups"))
	h := BackupsHandler(sm, mgr, al)
	do := func(role auth.Role, method, url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: role})
		req := httptest.NewRequest(method, url, nil)
		req.AddCookie(rec.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	info, err := mgr.Create(backup.KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, "/api/backups/copy?id="+info.ID); w.Code != http.StatusConflict {
		t.Fatalf("no destination: %d", w.Code)
	}
	off := filepath.Join(dir, "off")
	mgr.SetDestination(backup.DirDestination{Path: off})
	if w := do(auth.RoleViewer, http.MethodPost, "/api/backups/copy?id="+info.ID); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", w.Code)
	}
	if w := do(auth.RoleAdmin, http.MethodPost, "/api/backups/copy?id=../x"); w.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", w.Code)
	}
	w := do(auth.RoleAdmin, http.MethodPost, "/api/backups/copy?id="+info.ID)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("copy: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(off, info.ID, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	w = do(auth.RoleAdmin, http.MethodGet, "/api/backups")
	var list struct {
		Destination *backup.DestinationInfo `json:"destination"`
		Backups     []backup.Status         `json:"backups"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if list.Destination == nil || list.Destination.Kind != "dir" || list.Backups[0].Offsite == nil || !list.Backups[0].Offsite.OK {
		t.Fatalf("listing: %s", w.Body.String())
	}
	evs, _ := al.List(audit.Query{Action: audit.ActionBackupCopy})
	if len(evs) != 1 {
		t.Fatalf("copy must be audited once, got %d", len(evs))
	}
}
