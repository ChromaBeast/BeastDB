package handler

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// BackupsHandler serves the backup endpoints (admin only):
//
//	GET  /api/backups                 list, plus directory and schedule
//	POST /api/backups                 create a manual backup now
//	POST /api/backups/verify?id=      re-check one backup against its manifest
//	GET  /api/backups/download?id=    stream the bundle as a .tar
//
// Restore is deliberately not an endpoint: it stays an offline `beastctl restore`.
// Creating a backup pauses reads and writes for the time it takes to copy the
// data file, so the response says how long it took.
func BackupsHandler(sessions *auth.SessionManager, mgr *backup.Manager, al *audit.Log) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, role, err := sessions.ValidateSession(r)
		if err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if mgr == nil {
			http.Error(w, "Backups unavailable", http.StatusNotFound)
			return
		}
		ip := ClientIP(r)
		switch {
		case r.URL.Path == "/api/backups" && r.Method == http.MethodGet:
			list, err := mgr.List()
			if err != nil {
				http.Error(w, "Could not read the backup directory", http.StatusInternalServerError)
				return
			}
			every, keep := mgr.Schedule()
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, map[string]any{
				"dir": mgr.Dir(), "intervalMinutes": int(every / time.Minute), "keep": keep, "backups": list,
			})

		case r.URL.Path == "/api/backups" && r.Method == http.MethodPost:
			start := time.Now()
			info, err := mgr.Create(backup.KindManual, actor)
			switch {
			case errors.Is(err, backup.ErrBusy):
				http.Error(w, err.Error(), http.StatusConflict)
				return
			case errors.Is(err, backup.ErrNoSpace):
				http.Error(w, err.Error(), http.StatusInsufficientStorage)
				return
			case err != nil:
				http.Error(w, "Backup failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			_ = al.Record(actor, audit.ActionBackupCreate, info.ID, "manual · "+strconv.FormatInt(info.FileSize, 10)+" bytes", ip)
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]any{"backup": info, "tookMs": time.Since(start).Milliseconds()})

		case r.URL.Path == "/api/backups/verify" && r.Method == http.MethodPost:
			id := r.URL.Query().Get("id")
			res, err := mgr.Verify(id)
			if errors.Is(err, backup.ErrNotFound) {
				http.Error(w, "Backup not found", http.StatusNotFound)
				return
			}
			detail := "passed"
			if !res.OK {
				detail = "FAILED: " + res.Error
			}
			_ = al.Record(actor, audit.ActionBackupVerify, id, detail, ip)
			writeJSON(w, res)

		case r.URL.Path == "/api/backups/download" && r.Method == http.MethodGet:
			id := r.URL.Query().Get("id")
			dir, err := mgr.Path(id)
			if err != nil {
				http.Error(w, "Backup not found", http.StatusNotFound)
				return
			}
			_ = al.Record(actor, audit.ActionBackupDownload, id, "", ip)
			streamBundle(w, dir, id)

		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})
}

// streamBundle writes data.bin and manifest.json as an uncompressed tar. Headers
// are sent before any data, so a failure mid-stream truncates the archive; the
// client then fails to extract it rather than getting a silently short file.
func streamBundle(w http.ResponseWriter, dir, id string) {
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar"`, id))
	tw := tar.NewWriter(w)
	for _, name := range []string{"manifest.json", "data.bin"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return
		}
		if tw.WriteHeader(&tar.Header{Name: id + "/" + name, Mode: 0o640, Size: st.Size(), ModTime: st.ModTime()}) != nil {
			f.Close()
			return
		}
		_, err = io.Copy(tw, f)
		f.Close()
		if err != nil {
			return
		}
	}
	_ = tw.Close()
}
