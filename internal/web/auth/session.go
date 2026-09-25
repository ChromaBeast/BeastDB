package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	cookieName = "beastdb_session"
	sessionTTL = 8 * time.Hour
)

// SessionManager issues and validates HMAC-signed session cookies.
// The secret key must be at least 32 random bytes — never hardcoded in production.
type SessionManager struct {
	secret []byte
}

// NewSessionManager creates a SessionManager with the given HMAC secret.
func NewSessionManager(secret []byte) *SessionManager {
	return &SessionManager{secret: secret}
}

// GenerateSecret produces a cryptographically random 32-byte secret.
// Use this once at startup and persist the value across restarts.
func GenerateSecret() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// sessionPayload formats the cookie value: "username|role|expiry_unix".
func sessionPayload(username string, role Role, expiry time.Time) string {
	return fmt.Sprintf("%s|%s|%d", username, string(role), expiry.Unix())
}

// IssueSessionCookie signs a session payload and sets it as an HttpOnly cookie.
func (sm *SessionManager) IssueSessionCookie(w http.ResponseWriter, u *User) {
	expiry := time.Now().UTC().Add(sessionTTL)
	payload := sessionPayload(u.Username, u.Role, expiry)
	sig := sm.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,               // JS cannot read this cookie
		Secure:   true,               // Only sent over HTTPS
		SameSite: http.SameSiteStrictMode, // CSRF protection
	})
}

// ValidateSession reads and verifies the session cookie from the request.
// Returns the username and role if valid, error otherwise.
func (sm *SessionManager) ValidateSession(r *http.Request) (username string, role Role, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", fmt.Errorf("no session cookie")
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed session")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("malformed payload")
	}

	payload := string(payloadBytes)
	expected := sm.sign(payload)
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", "", fmt.Errorf("invalid session signature")
	}

	var expiryUnix int64
	var roleStr string
	_, scanErr := fmt.Sscanf(payload, "%s|%s|%d", &username, &roleStr, &expiryUnix)
	if scanErr != nil {
		// Sscanf with %s stops at whitespace; use split for robustness
		fields := strings.SplitN(payload, "|", 3)
		if len(fields) != 3 {
			return "", "", fmt.Errorf("malformed session payload")
		}
		username = fields[0]
		roleStr = fields[1]
		_, fmtErr := fmt.Sscanf(fields[2], "%d", &expiryUnix)
		if fmtErr != nil {
			return "", "", fmt.Errorf("malformed expiry")
		}
	}

	if time.Now().UTC().Unix() > expiryUnix {
		return "", "", fmt.Errorf("session expired")
	}

	return username, Role(roleStr), nil
}

// ClearSessionCookie removes the session cookie from the browser.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// sign produces a base64url-encoded HMAC-SHA256 signature over a payload.
func (sm *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, sm.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
