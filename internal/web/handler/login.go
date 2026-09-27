package handler

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// LoginRequest is the JSON body expected on POST /login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginHandler handles GET /login (serve page) and POST /login (authenticate).
func LoginHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			data, err := fs.ReadFile(d.StaticFS, "login.html")
			if err != nil {
				http.Error(w, "Login template missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
		case http.MethodPost:
			handleLoginPost(w, r, d)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}
}

// LogoutHandler handles POST /logout — clears the session cookie.
func LogoutHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		auth.ClearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// handleLoginPost validates credentials, issues a session cookie, then redirects.
func handleLoginPost(w http.ResponseWriter, r *http.Request, d *Deps) {
	ip := ClientIP(r)
	if d.RateLimiter != nil && !d.RateLimiter.Allow(ip) {
		http.Error(w, "Too many login attempts. Please wait a moment.", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 64KB max body
	var req LoginRequest

	ct := r.Header.Get("Content-Type")
	if ct == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
	} else {
		// Support HTML form and FormData submission
		_ = r.ParseMultipartForm(64 << 10)
		_ = r.ParseForm()
		req.Username = r.FormValue("username")
		req.Password = r.FormValue("password")
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	user, err := d.Users.Authenticate(req.Username, req.Password)
	if err != nil {
		if d.RateLimiter != nil {
			d.RateLimiter.RecordFailure(ip)
		}
		// Return 401 without revealing whether the user exists
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	d.Sessions.IssueSessionCookieForRequest(w, r, user)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
