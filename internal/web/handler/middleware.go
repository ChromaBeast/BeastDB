package handler

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// Deps holds shared dependencies injected into all HTTP handlers.
type Deps struct {
	Users       *auth.UserStore
	Sessions    *auth.SessionManager
	StaticFS    fs.FS
	RateLimiter *LoginRateLimiter
	Audit       *audit.Log // nil disables auditing
	// Require2FAAdmins reports whether admins must have a second factor (nil means no).
	Require2FAAdmins func() bool
	// Now overrides the clock for second-factor checks (tests); nil means time.Now.
	Now func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Deps) require2FAForAdmins() bool { return d.Require2FAAdmins != nil && d.Require2FAAdmins() }

// AuthMiddleware rejects unauthenticated requests to protected routes.
// On failure it redirects browsers to /login and returns 401 for API calls.
func AuthMiddleware(sessions *auth.SessionManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, err := sessions.ValidateSession(r)
		if err != nil {
			if isAPIRequest(r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AdminMiddleware additionally enforces the admin role on protected routes.
func AdminMiddleware(sessions *auth.SessionManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, role, err := sessions.ValidateSession(r)
		if err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAPIRequest returns true if the request targets an /api/ path.
func isAPIRequest(r *http.Request) bool {
	return len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/"
}
