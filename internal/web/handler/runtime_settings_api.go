package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/runtimecfg"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// RuntimeSettingsHandler serves /api/settings/runtime (admin only).
//
//	GET   -> {"values":{...},"ranges":{name:{min,max}}}
//	PATCH -> body {"wal_checkpoint_mb":128}; applies and persists the listed
//	         settings, records one audit entry, returns the same shape as GET.
//
// Only the settings in runtimecfg may change here; anything else is a 400.
func RuntimeSettingsHandler(sessions *auth.SessionManager, store *runtimecfg.Store, al *audit.Log) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, role, err := sessions.ValidateSession(r)
		if err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if store == nil {
			http.Error(w, "Runtime settings unavailable", http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPatch:
			r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
			var patch map[string]int
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil || len(patch) == 0 {
				http.Error(w, "Body must be a JSON object of setting names to whole numbers", http.StatusBadRequest)
				return
			}
			changes, err := store.Update(patch)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(changes) > 0 {
				sort.Slice(changes, func(i, j int) bool { return changes[i].Name < changes[j].Name })
				parts := make([]string, len(changes))
				for i, c := range changes {
					parts[i] = fmt.Sprintf("%s %d → %d", c.Name, c.From, c.To)
				}
				_ = al.Record(actor, audit.ActionSettingChange, "runtime", strings.Join(parts, "; "), ClientIP(r))
			}
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, map[string]any{"values": store.Get(), "ranges": runtimecfg.Ranges})
	})
}
