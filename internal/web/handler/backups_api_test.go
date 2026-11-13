package handler

import (
	"archive/tar"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func TestBackupsEndpointsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	for i := uint64(1); i <= 50; i++ {
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

	if w := do(auth.RoleViewer, http.MethodGet, "/api/backups"); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", w.Code)
	}
	anon := httptest.NewRecorder()
	h.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/backups", nil))
	if anon.Code != http.StatusForbidden {
		t.Fatalf("anonymous: %d", anon.Code)
	}

	w := do(auth.RoleAdmin, http.MethodPost, "/api/backups")
	var created struct {
		Backup backup.Info `json:"backup"`
	}
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &created) != nil || !backup.ValidID(created.Backup.ID) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := created.Backup.ID

	var list struct {
		Backups []backup.Status `json:"backups"`
	}
	w = do(auth.RoleAdmin, http.MethodGet, "/api/backups")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Backups) != 1 || list.Backups[0].ID != id || list.Backups[0].Verified == nil || !list.Backups[0].Verified.OK {
		t.Fatalf("a new backup is verified at creation: %+v", list)
	}

	if w = do(auth.RoleAdmin, http.MethodPost, "/api/backups/verify?id="+id); w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("verify: %d %s", w.Code, w.Body.String())
	}
	if w = do(auth.RoleAdmin, http.MethodPost, "/api/backups/verify?id=../../etc"); w.Code != http.StatusNotFound {
		t.Fatalf("path traversal id must 404, got %d", w.Code)
	}
	if w = do(auth.RoleAdmin, http.MethodGet, "/api/backups/download?id=bk-20200101-000000"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", w.Code)
	}

	w = do(auth.RoleAdmin, http.MethodGet, "/api/backups/download?id="+id)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-tar" {
		t.Fatalf("download: %d %v", w.Code, w.Header())
	}
	tr := tar.NewReader(w.Body)
	names := map[string]int64{}
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n, _ := io.Copy(io.Discard, tr)
		names[hd.Name] = n
	}
	if names[id+"/manifest.json"] == 0 || names[id+"/data.bin"] != created.Backup.FileSize {
		t.Fatalf("tar contents: %v (want data.bin %d bytes)", names, created.Backup.FileSize)
	}

	actions := map[string]bool{}
	entries, _ := al.List(audit.Query{})
	for _, e := range entries {
		actions[e.Action] = true
	}
	for _, a := range []string{audit.ActionBackupCreate, audit.ActionBackupVerify, audit.ActionBackupDownload} {
		if !actions[a] {
			t.Fatalf("missing audit entry %s: %+v", a, entries)
		}
	}

	var nilMgr = BackupsHandler(sm, nil, al)
	rec := httptest.NewRecorder()
	sm.IssueSessionCookie(rec, &auth.User{Username: "root", Role: auth.RoleAdmin})
	req := httptest.NewRequest(http.MethodGet, "/api/backups", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	nw := httptest.NewRecorder()
	nilMgr.ServeHTTP(nw, req)
	if nw.Code != http.StatusNotFound {
		t.Fatalf("nil manager: %d", nw.Code)
	}
}
