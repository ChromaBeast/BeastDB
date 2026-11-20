package handler

import (
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

type tfKV map[uint64][]byte

func (m tfKV) Put(k uint64, v []byte) error { m[k] = append([]byte(nil), v...); return nil }
func (m tfKV) Get(k uint64) ([]byte, bool, error) {
	v, ok := m[k]
	return v, ok, nil
}
func (m tfKV) Delete(k uint64) error { delete(m, k); return nil }

type tfEnv struct {
	t      *testing.T
	d      *Deps
	mux    *http.ServeMux
	clock  time.Time
	policy bool
}

func newTFEnv(t *testing.T) *tfEnv {
	t.Helper()
	al, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = al.Close() })
	e := &tfEnv{t: t, clock: time.Unix(1_800_000_000, 0)}
	sessions := auth.NewSessionManager(make([]byte, 32))
	e.d = &Deps{
		Users: auth.NewUserStore(tfKV{}), Sessions: sessions, RateLimiter: NewLoginRateLimiter(3, time.Minute), Audit: al,
		Now: func() time.Time { return e.clock }, Require2FAAdmins: func() bool { return e.policy },
	}
	for _, u := range []struct {
		n string
		r auth.Role
	}{{"ann", auth.RoleAdmin}, {"bob", auth.RoleViewer}} {
		if err := e.d.Users.CreateUser(u.n, "pw-"+u.n, u.r); err != nil {
			t.Fatal(err)
		}
	}
	e.mux = http.NewServeMux()
	e.mux.HandleFunc("/login", LoginHandler(&Deps{Users: e.d.Users, Sessions: sessions, RateLimiter: e.d.RateLimiter, Audit: al, Require2FAAdmins: e.d.Require2FAAdmins, StaticFS: nil}))
	e.mux.HandleFunc("/login/2fa", TwoFactorLoginHandler(e.d))
	e.mux.HandleFunc("/login/2fa/setup", TwoFactorLoginSetupHandler(e.d))
	e.mux.HandleFunc("/login/2fa/enable", TwoFactorLoginEnableHandler(e.d))
	acct := AuthMiddleware(sessions, TwoFactorAccountHandler(e.d))
	e.mux.Handle("/api/account/2fa", acct)
	e.mux.Handle("/api/account/2fa/", acct)
	e.mux.Handle("/api/users/2fa-reset", AdminMiddleware(sessions, TwoFactorResetHandler(e.d)))
	e.mux.Handle("/api/me", AuthMiddleware(sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })))
	return e
}

func (e *tfEnv) do(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

func (e *tfEnv) password(user string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, "/login", `{"username":"`+user+`","password":"pw-`+user+`"}`)
}

func (e *tfEnv) code(secret string) string {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		e.t.Fatal(err)
	}
	return auth.TOTPCode(raw, e.clock)
}

// enrol turns 2FA on for the user through the account API and returns secret and recovery codes.
func (e *tfEnv) enrol(user string) (string, []string) {
	e.t.Helper()
	sess := cookieNamed(e.password(user), "beastdb_session")
	if sess == nil {
		e.t.Fatal("no session")
	}
	var setup struct{ Secret, URI string }
	rec := e.do(http.MethodPost, "/api/account/2fa/setup", "", sess)
	_ = json.NewDecoder(rec.Body).Decode(&setup)
	if setup.Secret == "" || !strings.HasPrefix(setup.URI, "otpauth://totp/") {
		e.t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do(http.MethodPost, "/api/account/2fa/enable", `{"code":"`+e.code(setup.Secret)+`"}`, sess)
	var out struct{ RecoveryCodes []string }
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != 200 || len(out.RecoveryCodes) != 10 {
		e.t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	return setup.Secret, out.RecoveryCodes
}

func TestSessionOnlyAfterBothFactors(t *testing.T) {
	e := newTFEnv(t)
	secret, _ := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)

	rec := e.password("bob")
	if cookieNamed(rec, "beastdb_session") != nil {
		t.Fatal("password step issued a session")
	}
	pending := cookieNamed(rec, "beastdb_pending")
	if pending == nil || !strings.Contains(rec.Body.String(), `"twoFactor":"required"`) {
		t.Fatalf("expected pending challenge: %d %s", rec.Code, rec.Body.String())
	}
	// The half-authenticated state reaches no API and is no session.
	if r := e.do(http.MethodGet, "/api/me", "", pending); r.Code != 401 {
		t.Fatalf("pending cookie reached /api/me: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/api/me", "", &http.Cookie{Name: "beastdb_session", Value: pending.Value}); r.Code != 401 {
		t.Fatalf("pending value accepted as session: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/api/account/2fa", "", pending); r.Code != 401 {
		t.Fatalf("pending cookie reached account API: %d", r.Code)
	}
	// A code without the pending cookie does nothing.
	if r := e.do(http.MethodPost, "/login/2fa", `{"code":"`+e.code(secret)+`"}`); r.Code != 401 {
		t.Fatalf("code without pending cookie: %d", r.Code)
	}
	rec = e.do(http.MethodPost, "/login/2fa", `{"code":"`+e.code(secret)+`"}`, pending)
	sess := cookieNamed(rec, "beastdb_session")
	if rec.Code != 200 || sess == nil {
		t.Fatalf("second step: %d %s", rec.Code, rec.Body.String())
	}
	if r := e.do(http.MethodGet, "/api/me", "", sess); r.Code != 200 {
		t.Fatalf("session rejected: %d", r.Code)
	}
}

func TestReplayedCodeCannotSignInTwice(t *testing.T) {
	e := newTFEnv(t)
	secret, _ := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	code := e.code(secret)
	if r := e.do(http.MethodPost, "/login/2fa", `{"code":"`+code+`"}`, cookieNamed(e.password("bob"), "beastdb_pending")); r.Code != 200 {
		t.Fatalf("first use: %d", r.Code)
	}
	r := e.do(http.MethodPost, "/login/2fa", `{"code":"`+code+`"}`, cookieNamed(e.password("bob"), "beastdb_pending"))
	if r.Code != 401 || cookieNamed(r, "beastdb_session") != nil {
		t.Fatalf("replay accepted: %d", r.Code)
	}
}

func TestRecoveryCodeSignsInOnce(t *testing.T) {
	e := newTFEnv(t)
	_, codes := e.enrol("bob")
	use := func() int {
		p := cookieNamed(e.password("bob"), "beastdb_pending")
		return e.do(http.MethodPost, "/login/2fa", `{"code":"`+codes[0]+`"}`, p).Code
	}
	if c := use(); c != 200 {
		t.Fatalf("recovery code: %d", c)
	}
	if c := use(); c != 401 {
		t.Fatalf("recovery code reused: %d", c)
	}
}

func TestWrongCodesLockOutTheAccount(t *testing.T) {
	e := newTFEnv(t)
	secret, _ := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	pending := cookieNamed(e.password("bob"), "beastdb_pending")
	for i := 0; i < 3; i++ {
		if r := e.do(http.MethodPost, "/login/2fa", `{"code":"000000"}`, pending); r.Code != 401 {
			t.Fatalf("attempt %d: %d", i, r.Code)
		}
	}
	// Now even the right code is refused until the window passes.
	r := e.do(http.MethodPost, "/login/2fa", `{"code":"`+e.code(secret)+`"}`, pending)
	if r.Code != http.StatusTooManyRequests || cookieNamed(r, "beastdb_session") != nil {
		t.Fatalf("lockout not enforced: %d", r.Code)
	}
}

func TestAccountLimitHoldsAcrossAddresses(t *testing.T) {
	e := newTFEnv(t)
	secret, _ := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	pending := cookieNamed(e.password("bob"), "beastdb_pending")
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login/2fa", strings.NewReader(`{"code":"000000"}`))
		req.RemoteAddr = "10.9.9." + string(rune('1'+i)) + ":1"
		req.AddCookie(pending)
		e.mux.ServeHTTP(httptest.NewRecorder(), req)
	}
	req := httptest.NewRequest(http.MethodPost, "/login/2fa", strings.NewReader(`{"code":"`+e.code(secret)+`"}`))
	req.RemoteAddr = "10.9.9.200:1"
	req.AddCookie(pending)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a fresh address bypassed the per-account limit: %d", rec.Code)
	}
}

func TestDisableAndRegenerateNeedCodeAndAreAudited(t *testing.T) {
	e := newTFEnv(t)
	secret, codes := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	// Sign in fully to get a session.
	p := cookieNamed(e.password("bob"), "beastdb_pending")
	s := cookieNamed(e.do(http.MethodPost, "/login/2fa", `{"code":"`+codes[0]+`"}`, p), "beastdb_session")
	if r := e.do(http.MethodPost, "/api/account/2fa/disable", `{"code":"123456"}`, s); r.Code != 401 {
		t.Fatalf("disable with wrong code: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/api/account/2fa/recovery", `{"code":"`+codes[1]+`"}`, s); r.Code != 200 {
		t.Fatalf("regenerate: %d", r.Code)
	}
	e.clock = e.clock.Add(30 * time.Second)
	if r := e.do(http.MethodPost, "/api/account/2fa/disable", `{"code":"`+e.code(secret)+`"}`, s); r.Code != 204 {
		t.Fatalf("disable: %d", r.Code)
	}
	if r := e.password("bob"); cookieNamed(r, "beastdb_session") == nil {
		t.Fatal("after disable, password alone should sign in")
	}
	entries, _ := e.d.Audit.List(audit.Query{Limit: 100})
	seen := map[string]bool{}
	for _, en := range entries {
		seen[en.Action] = true
		blob := en.Actor + en.Target + en.Detail
		for _, c := range append(codes, secret, "123456") {
			if strings.Contains(blob, c) || strings.Contains(blob, strings.ReplaceAll(c, "-", "")) {
				t.Fatalf("audit entry leaks a code or secret: %+v", en)
			}
		}
	}
	for _, a := range []string{audit.Action2FAEnable, audit.Action2FADisable, audit.Action2FARecoveryGen, audit.ActionLogin2FAFailed, audit.ActionLoginOK} {
		if !seen[a] {
			t.Fatalf("missing audit action %s", a)
		}
	}
}

func TestAdminResetIsAdminOnlyNotSelfAndAudited(t *testing.T) {
	e := newTFEnv(t)
	e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	adminSess := cookieNamed(e.password("ann"), "beastdb_session")
	if err := e.d.Users.CreateUser("cat", "pw-cat", auth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer := cookieNamed(e.password("cat"), "beastdb_session")
	if r := e.do(http.MethodPost, "/api/users/2fa-reset", `{"username":"bob"}`, viewer); r.Code != 403 {
		t.Fatalf("viewer reset: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/api/users/2fa-reset", `{"username":"bob"}`); r.Code != 403 {
		t.Fatalf("anonymous reset: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/api/users/2fa-reset", `{"username":"ann"}`, adminSess); r.Code != 400 {
		t.Fatalf("self reset: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/api/users/2fa-reset", `{"username":"nobody"}`, adminSess); r.Code != 404 {
		t.Fatalf("unknown user: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/api/users/2fa-reset", `{"username":"bob"}`, adminSess); r.Code != 204 {
		t.Fatalf("reset: %d", r.Code)
	}
	if cookieNamed(e.password("bob"), "beastdb_session") == nil {
		t.Fatal("bob should sign in with the password alone after a reset")
	}
	entries, _ := e.d.Audit.List(audit.Query{Action: audit.Action2FAReset})
	if len(entries) != 1 || entries[0].Actor != "ann" || entries[0].Target != "bob" {
		t.Fatalf("reset audit: %+v", entries)
	}
}

func TestPolicyForcesAdminEnrolmentAtSignIn(t *testing.T) {
	e := newTFEnv(t)
	e.policy = true
	// Viewers are not affected.
	if cookieNamed(e.password("bob"), "beastdb_session") == nil {
		t.Fatal("policy must not apply to viewers")
	}
	rec := e.password("ann")
	pending := cookieNamed(rec, "beastdb_pending")
	if cookieNamed(rec, "beastdb_session") != nil || pending == nil || !strings.Contains(rec.Body.String(), `"twoFactor":"setup"`) {
		t.Fatalf("expected setup challenge: %s", rec.Body.String())
	}
	// A setup-state cookie cannot be used for the verify step, and vice versa.
	if r := e.do(http.MethodPost, "/login/2fa", `{"code":"123456"}`, pending); r.Code != 401 {
		t.Fatalf("setup cookie used for verify: %d", r.Code)
	}
	var setup struct{ Secret string }
	_ = json.NewDecoder(e.do(http.MethodPost, "/login/2fa/setup", "", pending).Body).Decode(&setup)
	if setup.Secret == "" {
		t.Fatal("no secret")
	}
	if r := e.do(http.MethodPost, "/login/2fa/enable", `{"code":"000000"}`, pending); r.Code != 401 {
		t.Fatalf("wrong confirm code: %d", r.Code)
	}
	r := e.do(http.MethodPost, "/login/2fa/enable", `{"code":"`+e.code(setup.Secret)+`"}`, pending)
	sess := cookieNamed(r, "beastdb_session")
	if r.Code != 200 || sess == nil || !strings.Contains(r.Body.String(), "recoveryCodes") {
		t.Fatalf("enrol at sign-in: %d %s", r.Code, r.Body.String())
	}
	// Admins cannot switch it off while the policy is on.
	e.clock = e.clock.Add(30 * time.Second)
	if r := e.do(http.MethodPost, "/api/account/2fa/disable", `{"code":"`+e.code(setup.Secret)+`"}`, sess); r.Code != 409 {
		t.Fatalf("admin disabled under policy: %d", r.Code)
	}
}

func TestSecretsNeverAppearInStatusOrLoginResponses(t *testing.T) {
	e := newTFEnv(t)
	secret, codes := e.enrol("bob")
	e.clock = e.clock.Add(30 * time.Second)
	rec := e.password("bob")
	p := cookieNamed(rec, "beastdb_pending")
	s := cookieNamed(e.do(http.MethodPost, "/login/2fa", `{"code":"`+codes[0]+`"}`, p), "beastdb_session")
	status := e.do(http.MethodGet, "/api/account/2fa", "", s)
	for _, body := range []string{rec.Body.String(), status.Body.String()} {
		if strings.Contains(body, secret) || strings.Contains(body, codes[1]) {
			t.Fatalf("secret material in response: %s", body)
		}
	}
	if !strings.Contains(status.Body.String(), `"recoveryRemaining":9`) {
		t.Fatalf("status: %s", status.Body.String())
	}
}
