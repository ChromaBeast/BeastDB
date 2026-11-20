package auth

import (
	"errors"
	"fmt"
	"time"
)

// Errors returned by the second-factor operations. They carry no secret data.
var (
	ErrInvalidCode    = errors.New("invalid code")
	ErrTOTPEnabled    = errors.New("two-factor is already enabled")
	ErrTOTPNotEnabled = errors.New("two-factor is not enabled")
	ErrNoPendingSetup = errors.New("no enrolment in progress")
	ErrUserNotFound   = errors.New("user not found")
	totpIssuerForURI  = "BeastDB"
)

// Second-factor methods reported on success.
const (
	MethodTOTP     = "totp"
	MethodRecovery = "recovery"
)

// load reads and decodes a user. Callers hold s.mu when they intend to write back.
func (s *UserStore) load(username string) (*User, error) {
	data, exists, err := s.engine.Get(sysUserKey(username))
	if err != nil {
		return nil, fmt.Errorf("reading user: %w", err)
	}
	if !exists {
		return nil, ErrUserNotFound
	}
	return UnmarshalUser(data)
}

func (s *UserStore) save(u *User) error {
	data, err := u.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling user: %w", err)
	}
	return s.engine.Put(sysUserKey(u.Username), data)
}

// TwoFactorStatus reports whether the user has 2FA on and how many recovery codes remain.
func (s *UserStore) TwoFactorStatus(username string) (enabled bool, recoveryLeft int, err error) {
	u, err := s.load(username)
	if err != nil {
		return false, 0, err
	}
	if !u.TwoFactorEnabled() {
		return false, 0, nil
	}
	return true, len(u.TOTP.RecoveryHashes), nil
}

// BeginTOTPSetup stores a fresh unconfirmed secret and returns it with its
// provisioning URI. It replaces any earlier unconfirmed secret.
func (s *UserStore) BeginTOTPSetup(username string) (secret, uri string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return "", "", err
	}
	if u.TwoFactorEnabled() {
		return "", "", ErrTOTPEnabled
	}
	secret, err = NewTOTPSecret()
	if err != nil {
		return "", "", err
	}
	if u.TOTP == nil {
		u.TOTP = &TOTPState{}
	}
	u.TOTP.PendingSecret = secret
	if err := s.save(u); err != nil {
		return "", "", err
	}
	return secret, OTPAuthURI(totpIssuerForURI, username, secret), nil
}

// ConfirmTOTP verifies a code against the pending secret, enables 2FA and
// returns the recovery codes (shown once; only hashes are kept).
func (s *UserStore) ConfirmTOTP(username, code string, now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return nil, err
	}
	if u.TwoFactorEnabled() {
		return nil, ErrTOTPEnabled
	}
	if u.TOTP == nil || u.TOTP.PendingSecret == "" {
		return nil, ErrNoPendingSetup
	}
	raw, err := decodeTOTPSecret(u.TOTP.PendingSecret)
	if err != nil {
		return nil, ErrNoPendingSetup
	}
	step, ok := verifyTOTP(raw, normalizeCode(code), now)
	if !ok {
		return nil, ErrInvalidCode
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	u.TOTP = &TOTPState{Secret: u.TOTP.PendingSecret, Enabled: true, LastStep: step, RecoveryHashes: hashes}
	if err := s.save(u); err != nil {
		return nil, err
	}
	return codes, nil
}

// checkSecondFactor validates a TOTP or recovery code against u and mutates u
// (advances LastStep, or removes the used recovery hash). The caller holds s.mu
// and persists u on success.
func checkSecondFactor(u *User, code string, now time.Time) (method string, err error) {
	if !u.TwoFactorEnabled() {
		return "", ErrTOTPNotEnabled
	}
	n := normalizeCode(code)
	if isSixDigits(n) {
		raw, derr := decodeTOTPSecret(u.TOTP.Secret)
		if derr != nil {
			return "", ErrInvalidCode
		}
		step, ok := verifyTOTP(raw, n, now)
		// A step is usable only once: it must be newer than the last accepted one.
		if !ok || step <= u.TOTP.LastStep {
			return "", ErrInvalidCode
		}
		u.TOTP.LastStep = step
		return MethodTOTP, nil
	}
	if len(n) != recoveryCodeLen {
		return "", ErrInvalidCode
	}
	i := matchRecovery(u.TOTP.RecoveryHashes, n)
	if i < 0 {
		return "", ErrInvalidCode
	}
	u.TOTP.RecoveryHashes = append(append([]string{}, u.TOTP.RecoveryHashes[:i]...), u.TOTP.RecoveryHashes[i+1:]...)
	return MethodRecovery, nil
}

// VerifySecondFactor checks a TOTP or recovery code for sign-in and consumes it.
func (s *UserStore) VerifySecondFactor(username, code string, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return "", err
	}
	method, err := checkSecondFactor(u, code, now)
	if err != nil {
		return "", err
	}
	if err := s.save(u); err != nil {
		return "", err
	}
	return method, nil
}

// DisableTOTP turns 2FA off after a valid current or recovery code.
func (s *UserStore) DisableTOTP(username, code string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return err
	}
	if _, err := checkSecondFactor(u, code, now); err != nil {
		return err
	}
	u.TOTP = nil
	return s.save(u)
}

// RegenerateRecoveryCodes replaces all recovery codes after a valid code.
func (s *UserStore) RegenerateRecoveryCodes(username, code string, now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return nil, err
	}
	if _, err := checkSecondFactor(u, code, now); err != nil {
		return nil, err
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	u.TOTP.RecoveryHashes = hashes
	if err := s.save(u); err != nil {
		return nil, err
	}
	return codes, nil
}

// ResetTOTP clears a user's second factor without a code. Only for the
// admin-reset endpoint and the operator's startup flag.
func (s *UserStore) ResetTOTP(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.load(username)
	if err != nil {
		return err
	}
	u.TOTP = nil
	return s.save(u)
}
