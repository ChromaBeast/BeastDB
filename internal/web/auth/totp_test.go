package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memKV map[uint64][]byte

func (m memKV) Put(k uint64, v []byte) error { m[k] = append([]byte(nil), v...); return nil }
func (m memKV) Get(k uint64) ([]byte, bool, error) {
	v, ok := m[k]
	return v, ok, nil
}
func (m memKV) Delete(k uint64) error { delete(m, k); return nil }

// RFC 4226 Appendix D, secret "12345678901234567890".
func TestHOTPVectors(t *testing.T) {
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	secret := []byte("12345678901234567890")
	for i, w := range want {
		if got := hotp(secret, uint64(i)); got != w {
			t.Fatalf("counter %d: got %s want %s", i, got, w)
		}
	}
}

// RFC 6238 Appendix B, SHA-1. The RFC lists 8 digits; the 6-digit code is the
// same truncated value modulo 10^6, i.e. the last six digits.
func TestTOTPRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, eight := range map[int64]string{
		59: "94287082", 1111111109: "07081804", 1111111111: "14050471",
		1234567890: "89005924", 2000000000: "69279037", 20000000000: "65353130",
	} {
		if got := TOTPCode(secret, time.Unix(unix, 0)); got != eight[2:] {
			t.Fatalf("t=%d: got %s want %s", unix, got, eight[2:])
		}
	}
}

func TestVerifyTOTPWindow(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111109, 0)
	for _, d := range []int{-1, 0, 1} {
		code := TOTPCode(secret, now.Add(time.Duration(d*30)*time.Second))
		if _, ok := verifyTOTP(secret, code, now); !ok {
			t.Fatalf("step offset %d should be accepted", d)
		}
	}
	for _, d := range []int{-2, 2} {
		code := TOTPCode(secret, now.Add(time.Duration(d*30)*time.Second))
		if _, ok := verifyTOTP(secret, code, now); ok {
			t.Fatalf("step offset %d should be rejected", d)
		}
	}
}

func TestOTPAuthURI(t *testing.T) {
	u := OTPAuthURI("BeastDB", "ann", "JBSWY3DPEHPK3PXP")
	for _, part := range []string{"otpauth://totp/BeastDB:ann?", "secret=JBSWY3DPEHPK3PXP", "issuer=BeastDB", "digits=6", "period=30", "algorithm=SHA1"} {
		if !strings.Contains(u, part) {
			t.Fatalf("%q missing %q", u, part)
		}
	}
}

func enrolled(t *testing.T) (*UserStore, []byte, []string, time.Time) {
	t.Helper()
	s := NewUserStore(memKV{})
	if err := s.CreateUser("ann", "pw-ann", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	b32secret, _, err := s.BeginTOTPSetup("ann")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := decodeTOTPSecret(b32secret)
	now := time.Unix(1_800_000_000, 0)
	codes, err := s.ConfirmTOTP("ann", TOTPCode(raw, now), now)
	if err != nil {
		t.Fatal(err)
	}
	return s, raw, codes, now
}

func TestEnrolmentNeedsAValidCode(t *testing.T) {
	s := NewUserStore(memKV{})
	_ = s.CreateUser("ann", "pw", RoleViewer)
	if _, err := s.ConfirmTOTP("ann", "123456", time.Now()); !errors.Is(err, ErrNoPendingSetup) {
		t.Fatalf("confirm without setup: %v", err)
	}
	_, _, _ = s.BeginTOTPSetup("ann")
	if _, err := s.ConfirmTOTP("ann", "000000", time.Unix(1_800_000_000, 0)); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("wrong code: %v", err)
	}
	if on, _, _ := s.TwoFactorStatus("ann"); on {
		t.Fatal("2FA must stay off until a code is confirmed")
	}
}

func TestReplayOfUsedStepIsRejected(t *testing.T) {
	s, raw, _, now := enrolled(t)
	// The code used to confirm enrolment is already spent.
	if _, err := s.VerifySecondFactor("ann", TOTPCode(raw, now), now); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("enrolment code replayed: %v", err)
	}
	next := now.Add(30 * time.Second)
	if m, err := s.VerifySecondFactor("ann", TOTPCode(raw, next), next); err != nil || m != MethodTOTP {
		t.Fatalf("fresh step: %v %s", err, m)
	}
	if _, err := s.VerifySecondFactor("ann", TOTPCode(raw, next), next); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("same step twice: %v", err)
	}
	// An older step still inside the window must not be accepted after a newer one.
	if _, err := s.VerifySecondFactor("ann", TOTPCode(raw, now), next); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("older step after newer: %v", err)
	}
}

func TestRecoveryCodeIsSingleUseAndStoredHashed(t *testing.T) {
	kv := memKV{}
	s := NewUserStore(kv)
	_ = s.CreateUser("ann", "pw", RoleViewer)
	b, _, _ := s.BeginTOTPSetup("ann")
	raw, _ := decodeTOTPSecret(b)
	now := time.Unix(1_800_000_000, 0)
	codes, err := s.ConfirmTOTP("ann", TOTPCode(raw, now), now)
	if err != nil || len(codes) != recoveryCodeCount {
		t.Fatalf("codes: %v %d", err, len(codes))
	}
	for _, v := range kv {
		for _, c := range codes {
			if strings.Contains(string(v), c) || strings.Contains(string(v), strings.ReplaceAll(c, "-", "")) {
				t.Fatal("a recovery code is stored in clear")
			}
		}
	}
	if m, err := s.VerifySecondFactor("ann", strings.ToLower(codes[0]), now); err != nil || m != MethodRecovery {
		t.Fatalf("first use: %v %s", err, m)
	}
	if _, err := s.VerifySecondFactor("ann", codes[0], now); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("reuse: %v", err)
	}
	if _, left, _ := s.TwoFactorStatus("ann"); left != recoveryCodeCount-1 {
		t.Fatalf("remaining %d", left)
	}
	if _, err := s.VerifySecondFactor("ann", "AAAAA-AAAAA", now); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("unknown recovery code: %v", err)
	}
}

func TestDisableAndRegenerateNeedACode(t *testing.T) {
	s, raw, codes, now := enrolled(t)
	if err := s.DisableTOTP("ann", "000000", now.Add(30*time.Second)); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("disable with bad code: %v", err)
	}
	fresh, err := s.RegenerateRecoveryCodes("ann", codes[0], now)
	if err != nil || len(fresh) != recoveryCodeCount {
		t.Fatalf("regenerate: %v", err)
	}
	if _, err := s.VerifySecondFactor("ann", codes[1], now); !errors.Is(err, ErrInvalidCode) {
		t.Fatal("old recovery codes must stop working after regeneration")
	}
	next := now.Add(60 * time.Second)
	if err := s.DisableTOTP("ann", TOTPCode(raw, next), next); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if on, _, _ := s.TwoFactorStatus("ann"); on {
		t.Fatal("still enabled")
	}
}

func TestSetUserPasswordKeepsSecondFactor(t *testing.T) {
	s, _, _, _ := enrolled(t)
	if err := s.SetUserPassword("ann", "new-password", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if on, _, _ := s.TwoFactorStatus("ann"); !on {
		t.Fatal("a password sync must not silently remove 2FA")
	}
	if err := s.ResetTOTP("ann"); err != nil {
		t.Fatal(err)
	}
	if on, _, _ := s.TwoFactorStatus("ann"); on {
		t.Fatal("reset did not clear 2FA")
	}
}

func TestPendingCookieIsNotASession(t *testing.T) {
	sm := NewSessionManager(make([]byte, 32))
	rec := httptest.NewRecorder()
	sm.IssuePendingCookie(rec, nil, "ann", PendingVerify)
	c := rec.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	if name, st, err := sm.ValidatePending(req); err != nil || name != "ann" || st != PendingVerify {
		t.Fatalf("pending: %v %s %s", err, name, st)
	}
	if _, _, err := sm.ValidateSession(req); err == nil {
		t.Fatal("pending cookie validated as a session")
	}
	// Same value presented under the session cookie name must fail too.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: c.Value})
	if _, _, err := sm.ValidateSession(req); err == nil {
		t.Fatal("pending token accepted as session cookie")
	}
}
