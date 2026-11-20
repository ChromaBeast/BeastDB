package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// TwoFactorAccountHandler serves /api/account/2fa/... for the signed-in user:
//
//	GET  /api/account/2fa            -> {"enabled":bool,"recoveryRemaining":n,"requiredByPolicy":bool}
//	POST /api/account/2fa/setup      -> {"secret","uri"}            (not yet enabled)
//	POST /api/account/2fa/enable     {"code"} -> {"recoveryCodes"}  (confirms the setup)
//	POST /api/account/2fa/disable    {"code"}  TOTP or recovery code
//	POST /api/account/2fa/recovery   {"code"} -> {"recoveryCodes"}  (replaces all codes)
//
// Secrets and recovery codes appear only in the responses that create them.
func TwoFactorAccountHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, role, err := d.Sessions.ValidateSession(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ip := ClientIP(r)
		sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/account/2fa"), "/")
		if sub == "" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			enabled, left, err := d.Users.TwoFactorStatus(username)
			if err != nil {
				http.Error(w, "Could not read account", http.StatusInternalServerError)
				return
			}
			writeNoStoreJSON(w, map[string]any{"enabled": enabled, "recoveryRemaining": left, "requiredByPolicy": role == auth.RoleAdmin && d.require2FAForAdmins()})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		switch sub {
		case "setup":
			secret, uri, err := d.Users.BeginTOTPSetup(username)
			if errors.Is(err, auth.ErrTOTPEnabled) {
				http.Error(w, "Two-factor is already enabled", http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, "Could not start setup", http.StatusInternalServerError)
				return
			}
			writeNoStoreJSON(w, map[string]any{"secret": secret, "uri": uri})
		case "enable", "disable", "recovery":
			if !twoFactorAllowed(d, w, ip, username) {
				return
			}
			code, ok := decodeCode(w, r)
			if !ok {
				return
			}
			if sub == "disable" && role == auth.RoleAdmin && d.require2FAForAdmins() {
				http.Error(w, "Two-factor is required for administrators", http.StatusConflict)
				return
			}
			var codes []string
			var action, detail string
			switch sub {
			case "enable":
				codes, err = d.Users.ConfirmTOTP(username, code, d.now())
				action, detail = audit.Action2FAEnable, "enrolled in Studio"
			case "disable":
				err = d.Users.DisableTOTP(username, code, d.now())
				action, detail = audit.Action2FADisable, "disabled by the account owner"
			default:
				codes, err = d.Users.RegenerateRecoveryCodes(username, code, d.now())
				action, detail = audit.Action2FARecoveryGen, "recovery codes replaced"
			}
			switch {
			case err == nil:
			case errors.Is(err, auth.ErrInvalidCode):
				twoFactorFailed(d, ip, username, sub+" code rejected")
				http.Error(w, "Invalid code", http.StatusUnauthorized)
				return
			case errors.Is(err, auth.ErrTOTPEnabled), errors.Is(err, auth.ErrTOTPNotEnabled), errors.Is(err, auth.ErrNoPendingSetup):
				http.Error(w, err.Error(), http.StatusConflict)
				return
			default:
				http.Error(w, "Could not update two-factor", http.StatusInternalServerError)
				return
			}
			_ = d.Audit.Record(username, action, username, detail, ip)
			if codes != nil {
				writeNoStoreJSON(w, map[string]any{"recoveryCodes": codes})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

// TwoFactorResetHandler serves POST /api/users/2fa-reset {"username"} for admins:
// it removes another user's second factor (lost device and recovery codes).
// Resetting your own account is refused; use disable, which needs a code.
func TwoFactorResetHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, role, err := d.Sessions.ValidateSession(r)
		if err != nil || role != auth.RoleAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		var body struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Username) == "" {
			http.Error(w, "A username is required", http.StatusBadRequest)
			return
		}
		target := strings.TrimSpace(body.Username)
		if target == actor {
			http.Error(w, "Use disable on your own account; it needs a current code", http.StatusBadRequest)
			return
		}
		switch err := d.Users.ResetTOTP(target); {
		case err == nil:
		case errors.Is(err, auth.ErrUserNotFound):
			http.Error(w, "No such user", http.StatusNotFound)
			return
		default:
			http.Error(w, "Could not reset two-factor", http.StatusInternalServerError)
			return
		}
		_ = d.Audit.Record(actor, audit.Action2FAReset, target, "second factor removed by an administrator", ClientIP(r))
		w.WriteHeader(http.StatusNoContent)
	})
}
