package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default; HMAC-SHA-1 is not affected by SHA-1 collisions.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpDigits    = 6
	totpPeriod    = 30 // seconds
	totpWindow    = 1  // accepted steps either side of the current one
	totpSecretLen = 20 // 160 bits, the HMAC-SHA-1 block recommendation

	recoveryCodeCount = 10
	recoveryCodeLen   = 10
)

var (
	b32           = base32.StdEncoding.WithPadding(base32.NoPadding)
	recoveryAlpha = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 symbols, no 0/O/1/I
)

// TOTPState is the per-user second-factor record kept inside the User.
type TOTPState struct {
	Secret         string   `json:"secret,omitempty"`          // base32, only once enabled
	Enabled        bool     `json:"enabled"`                   //
	LastStep       int64    `json:"last_step,omitempty"`       // newest accepted time step (replay guard)
	PendingSecret  string   `json:"pending_secret,omitempty"`  // enrolment started, not confirmed
	RecoveryHashes []string `json:"recovery_hashes,omitempty"` // hex SHA-256 of unused codes
}

// hotp is RFC 4226 with SHA-1 and 6 digits.
func hotp(secret []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, bin%1000000)
}

// totpStep is the RFC 6238 time step for t.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// TOTPCode returns the code for the time step containing t.
func TOTPCode(secret []byte, t time.Time) string { return hotp(secret, uint64(totpStep(t))) }

// verifyTOTP checks code against the window around now. It returns the matched
// step, or ok=false. Every candidate is compared in constant time and the loop
// never exits early, so timing does not reveal which step (if any) matched.
func verifyTOTP(secret []byte, code string, now time.Time) (step int64, ok bool) {
	cur := totpStep(now)
	var hit int64
	var found int
	for d := int64(-totpWindow); d <= totpWindow; d++ {
		s := cur + d
		if s < 0 {
			continue
		}
		want := hotp(secret, uint64(s))
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			hit, found = s, 1
		}
	}
	return hit, found == 1
}

// NewTOTPSecret returns a fresh random secret as unpadded base32.
func NewTOTPSecret() (string, error) {
	b := make([]byte, totpSecretLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating TOTP secret: %w", err)
	}
	return b32.EncodeToString(b), nil
}

func decodeTOTPSecret(s string) ([]byte, error) { return b32.DecodeString(s) }

// OTPAuthURI builds the otpauth:// provisioning URI authenticator apps import.
func OTPAuthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// normalizeCode strips spaces and dashes so "123 456" and "abcde-fghjk" are accepted.
func normalizeCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	return strings.ReplaceAll(s, "-", "")
}

func isSixDigits(s string) bool {
	if len(s) != totpDigits {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func hashRecovery(normalized string) string {
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// newRecoveryCodes returns the display codes ("ABCDE-FGHJK") and their hashes.
func newRecoveryCodes() (codes, hashes []string, err error) {
	for i := 0; i < recoveryCodeCount; i++ {
		raw := make([]byte, recoveryCodeLen)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, fmt.Errorf("generating recovery code: %w", err)
		}
		c := make([]byte, recoveryCodeLen)
		for j, b := range raw {
			c[j] = recoveryAlpha[int(b)&31] // 256 is a multiple of 32: no modulo bias
		}
		codes = append(codes, string(c[:5])+"-"+string(c[5:]))
		hashes = append(hashes, hashRecovery(string(c)))
	}
	return codes, hashes, nil
}

// matchRecovery returns the index of the hash equal to the normalized code, or -1.
// It scans every hash with constant-time compares.
func matchRecovery(hashes []string, normalized string) int {
	want := hashRecovery(normalized)
	idx := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			idx = i
		}
	}
	return idx
}
