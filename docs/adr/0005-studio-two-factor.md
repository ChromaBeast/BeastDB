# ADR 0005: TOTP two-factor sign-in for Studio accounts

**Status:** accepted and implemented, 20 November 2026.

## Context

Studio accounts are local users (admin or viewer) with an argon2id password hash in the user store. A leaked or guessed password is the only thing between an attacker and an admin session. This adds an optional second factor. It is **not** SSO, not federation and not a replacement for network controls (bind Studio to a trusted interface, put TLS in front of it).

## Decision

### Algorithm

TOTP per RFC 6238 on top of HOTP (RFC 4226): HMAC-SHA-1, 6 digits, 30 second step, 160-bit random secret, standard library only (`crypto/hmac`, `crypto/sha1`, `crypto/rand`, `encoding/base32`). SHA-1 is what authenticator apps universally support; HMAC-SHA-1 is not affected by the collision attacks on SHA-1. A code is accepted for the current step and one step either side (about 90 seconds of clock skew in total). The implementation is checked against the RFC 6238 Appendix B SHA-1 vectors (8-digit values, truncated to 6 for the product).

### Storage

The user record gains an optional `totp` object: the base32 secret, an enabled flag, the last accepted time step, an unconfirmed enrolment secret, and the SHA-256 hashes of the unused recovery codes. User records live in the engine's keyspace, so they get exactly the protection the data file has:

- **Encryption at rest on** (`-encryption-key-file`, ADR 0003): the secret is inside AES-256-GCM encrypted pages, the WAL and backups, under the engine's key. There is no second, separate key.
- **Encryption at rest off:** the secret is stored in plaintext in the data file, the WAL and every backup. Anyone who can read those files can generate valid codes. A TOTP secret has to be recoverable by the server, so it cannot be hashed. Turn on encryption at rest if that matters.

Recovery codes are 10 random characters from a 32-letter alphabet (50 bits), shown once, stored only as SHA-256 hashes, removed on use. They are high-entropy random values, so a plain hash is adequate; guessing is bounded by the rate limit.

### Sign-in flow

1. `POST /login` checks the password. For an account with 2FA enabled it issues **no session cookie**. It sets a short-lived (5 minute) `beastdb_pending` cookie, `HttpOnly`, `SameSite=Strict`, scoped to `/login/2fa`, signed with the session secret under a different domain-separation prefix, and answers `{"twoFactor":"required"}`.
2. `POST /login/2fa` takes a TOTP or recovery code, requires the pending cookie, and only then issues the normal session cookie.

Because `ValidateSession` reads only the session cookie and the pending cookie has another name, another signature domain and another path, the half-authenticated state cannot reach any other route. Tests assert that.

### Replay and brute force

- The last accepted time step is stored with the user. A code is accepted only for a step strictly greater than it, so a code (or an earlier code from the window) cannot be used twice. The check and update run under one mutex.
- Code comparison is constant time and does not exit early across the window or the recovery list.
- Failed second-factor attempts are counted in the existing sign-in limiter (`LoginRateLimiter`) twice: per client IP, and per account under the key `2fa:<username>`, using the same live-configurable limit (`login_max_attempts` per `login_window_seconds`). The per-account key stops guessing from many addresses; the cost is that someone who already knows a password can lock that account's second step for one window.

### Enrolment and management

In Studio (Settings, Security) a signed-in user sees the `otpauth://` URI and the base32 setup key (no QR code; adding a QR library for 160 bits of text is not justified), types a code from the app, and only then is 2FA enabled and ten recovery codes shown. Disabling 2FA and regenerating recovery codes need a current TOTP or recovery code. An admin can reset another user's 2FA (not their own); that clears the secret and codes and is audited.

### Policy

Runtime setting `require_2fa_admins` (0 or 1, default 0, changed under Settings, Security, saved in `runtime-settings.json`). When 1, an admin without 2FA who passes the password step gets the pending cookie in "setup" state and must enrol (secret, confirm with a code, save recovery codes) on the sign-in page before a session is issued. Admins cannot disable their own 2FA while it is on. Sessions already issued stay valid until they expire (8 hours).

### Lockout recovery

If the only admin loses the device and the recovery codes, starting the server with `-reset-admin-2fa` clears the `admin` account's 2FA (the operator has host access, which already beats the password). Setting the admin password with `-admin-password` keeps existing 2FA. Other users are reset by an admin in Studio.

### Audit

New actions: `login.2fa.failed`, `2fa.enable`, `2fa.disable`, `2fa.recovery.regenerate`, `2fa.reset`. `login.ok` is recorded only after the second factor and says which method was used. Entries never contain codes, secrets or recovery codes.

## Not in this change

WebAuthn/passkeys, SMS or email codes, remembered devices, TOTP for gRPC clients or API tokens, per-session revocation, SHA-256/512 TOTP variants, QR rendering.
