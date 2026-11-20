package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

func writeNoStoreJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, v)
}

// decodeCode reads {"code": "..."} with a small body cap.
func decodeCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, "A code is required", http.StatusBadRequest)
		return "", false
	}
	return body.Code, true
}

func twoFactorKey(username string) string { return "2fa:" + username }

// twoFactorAllowed applies the sign-in limiter per client address and per
// account; the account key stops guessing spread over many addresses.
func twoFactorAllowed(d *Deps, w http.ResponseWriter, ip, username string) bool {
	if d.RateLimiter != nil && (!d.RateLimiter.Allow(ip) || !d.RateLimiter.Allow(twoFactorKey(username))) {
		http.Error(w, "Too many attempts. Please wait a moment.", http.StatusTooManyRequests)
		return false
	}
	return true
}

// twoFactorFailed counts the failure on both limiter keys and audits it. The
// detail never contains the submitted code.
func twoFactorFailed(d *Deps, ip, username, detail string) {
	if d.RateLimiter != nil {
		d.RateLimiter.RecordFailure(ip)
		d.RateLimiter.RecordFailure(twoFactorKey(username))
	}
	_ = d.Audit.Record(username, audit.ActionLogin2FAFailed, "", detail, ip)
}

// pendingUser returns the user named by the pending-sign-in cookie, or answers 401.
func pendingUser(d *Deps, w http.ResponseWriter, r *http.Request, want string) (string, bool) {
	name, state, err := d.Sessions.ValidatePending(r)
	if err != nil || state != want {
		auth.ClearPendingCookie(w)
		http.Error(w, "Sign in again", http.StatusUnauthorized)
		return "", false
	}
	return name, true
}

func finishTwoFactorLogin(d *Deps, w http.ResponseWriter, r *http.Request, username, detail string) bool {
	user, err := d.Users.GetUser(username)
	if err != nil {
		auth.ClearPendingCookie(w)
		http.Error(w, "Sign in again", http.StatusUnauthorized)
		return false
	}
	_ = d.Audit.Record(user.Username, audit.ActionLoginOK, "", "role "+string(user.Role)+" · "+detail, ClientIP(r))
	auth.ClearPendingCookie(w)
	d.Sessions.IssueSessionCookieForRequest(w, r, user)
	return true
}

// TwoFactorLoginHandler handles POST /login/2fa: the second step of sign-in.
// It needs the pending cookie from a correct password and a TOTP or recovery
// code, and only then issues the session cookie.
func TwoFactorLoginHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		username, ok := pendingUser(d, w, r, auth.PendingVerify)
		if !ok {
			return
		}
		ip := ClientIP(r)
		if !twoFactorAllowed(d, w, ip, username) {
			return
		}
		code, ok := decodeCode(w, r)
		if !ok {
			return
		}
		method, err := d.Users.VerifySecondFactor(username, code, d.now())
		switch {
		case err == nil:
		case errors.Is(err, auth.ErrInvalidCode):
			twoFactorFailed(d, ip, username, "code rejected")
			http.Error(w, "Invalid code", http.StatusUnauthorized)
			return
		default:
			auth.ClearPendingCookie(w)
			http.Error(w, "Sign in again", http.StatusUnauthorized)
			return
		}
		if finishTwoFactorLogin(d, w, r, username, "2fa "+method) {
			writeNoStoreJSON(w, map[string]any{"ok": true})
		}
	}
}

// TwoFactorLoginSetupHandler handles POST /login/2fa/setup: when policy forces
// an admin to enrol, it creates the secret for the account named by the pending cookie.
func TwoFactorLoginSetupHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		username, ok := pendingUser(d, w, r, auth.PendingSetup)
		if !ok {
			return
		}
		secret, uri, err := d.Users.BeginTOTPSetup(username)
		if err != nil {
			http.Error(w, "Could not start setup", http.StatusConflict)
			return
		}
		writeNoStoreJSON(w, map[string]any{"secret": secret, "uri": uri})
	}
}

// TwoFactorLoginEnableHandler handles POST /login/2fa/enable: confirms the code,
// turns 2FA on, returns the recovery codes once and signs the user in.
func TwoFactorLoginEnableHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		username, ok := pendingUser(d, w, r, auth.PendingSetup)
		if !ok {
			return
		}
		ip := ClientIP(r)
		if !twoFactorAllowed(d, w, ip, username) {
			return
		}
		code, ok := decodeCode(w, r)
		if !ok {
			return
		}
		codes, err := d.Users.ConfirmTOTP(username, code, d.now())
		switch {
		case err == nil:
		case errors.Is(err, auth.ErrInvalidCode):
			twoFactorFailed(d, ip, username, "setup code rejected")
			http.Error(w, "Invalid code", http.StatusUnauthorized)
			return
		default:
			http.Error(w, "Could not enable two-factor", http.StatusConflict)
			return
		}
		_ = d.Audit.Record(username, audit.Action2FAEnable, username, "enrolled at sign-in (required for admins)", ip)
		if finishTwoFactorLogin(d, w, r, username, "2fa enrolment") {
			writeNoStoreJSON(w, map[string]any{"recoveryCodes": codes})
		}
	}
}
