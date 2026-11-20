package auth

import (
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	pendingCookieName = "beastdb_pending"
	pendingPath       = "/login/2fa"
	pendingTTL        = 5 * time.Minute
)

// Pending sign-in states.
const (
	PendingVerify = "verify" // password accepted; a code is required
	PendingSetup  = "setup"  // password accepted; policy requires enrolling first
)

// IssuePendingCookie records that username passed the password step. It is not
// a session: ValidateSession ignores it, it is signed under a separate prefix,
// it lives under /login/2fa only, and it expires in five minutes.
func (sm *SessionManager) IssuePendingCookie(w http.ResponseWriter, r *http.Request, username, state string) {
	exp := time.Now().UTC().Add(pendingTTL)
	payload := base64.RawURLEncoding.EncodeToString([]byte(username)) + "|" + state + "|" + strconv.FormatInt(exp.Unix(), 10)
	secure := r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
	http.SetCookie(w, &http.Cookie{
		Name: pendingCookieName, Value: base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sm.sign("pending:"+payload),
		Path: pendingPath, Expires: exp, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

// ValidatePending returns the username and state from a valid pending cookie.
func (sm *SessionManager) ValidatePending(r *http.Request) (username, state string, err error) {
	c, err := r.Cookie(pendingCookieName)
	if err != nil {
		return "", "", fmt.Errorf("no pending sign-in")
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed pending sign-in")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("malformed pending sign-in")
	}
	payload := string(raw)
	if !hmac.Equal([]byte(sm.sign("pending:"+payload)), []byte(parts[1])) {
		return "", "", fmt.Errorf("invalid pending signature")
	}
	f := strings.Split(payload, "|")
	if len(f) != 3 {
		return "", "", fmt.Errorf("malformed pending sign-in")
	}
	exp, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || time.Now().UTC().Unix() > exp {
		return "", "", fmt.Errorf("pending sign-in expired")
	}
	name, err := base64.RawURLEncoding.DecodeString(f[0])
	if err != nil || (f[1] != PendingVerify && f[1] != PendingSetup) {
		return "", "", fmt.Errorf("malformed pending sign-in")
	}
	return string(name), f[1], nil
}

// ClearPendingCookie removes the pending cookie.
func ClearPendingCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: pendingCookieName, Value: "", Path: pendingPath, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}
