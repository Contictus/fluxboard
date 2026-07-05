package authuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/aesgcm"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/totp"
)

// --- additional fakes ------------------------------------------------------

type fakeLimiter struct {
	deny    bool
	resets  int
	allowed int
}

func (f *fakeLimiter) Allow(context.Context, string) (bool, time.Duration, error) {
	if f.deny {
		return false, time.Minute, nil
	}
	f.allowed++
	return true, 0, nil
}
func (f *fakeLimiter) Reset(context.Context, string) error { f.resets++; return nil }

type fakeRecovery struct {
	codes   map[string]bool // hash-hex -> used
	replace int
}

func (f *fakeRecovery) Replace(_ context.Context, _ string, hashes [][]byte) error {
	f.replace++
	f.codes = map[string]bool{}
	for _, h := range hashes {
		f.codes[string(h)] = false
	}
	return nil
}
func (f *fakeRecovery) Consume(_ context.Context, _ string, hash []byte) (bool, error) {
	if used, ok := f.codes[string(hash)]; ok && !used {
		f.codes[string(hash)] = true
		return true, nil
	}
	return false, nil
}

type fakeMailer struct{ verify, reset int }

func (f *fakeMailer) SendEmailVerify(context.Context, string, string) error   { f.verify++; return nil }
func (f *fakeMailer) SendPasswordReset(context.Context, string, string) error { f.reset++; return nil }

type fakeOTT struct {
	created  []*auth.OneTimeToken
	consume  *auth.OneTimeToken
	consumed bool
}

func (f *fakeOTT) Create(_ context.Context, t *auth.OneTimeToken) error {
	f.created = append(f.created, t)
	return nil
}
func (f *fakeOTT) Consume(context.Context, auth.TokenPurpose, []byte, time.Time) (*auth.OneTimeToken, error) {
	if f.consume == nil {
		return nil, domain.ErrNotFound
	}
	f.consumed = true
	return f.consume, nil
}

type stubProvider struct {
	user auth.OAuthUser
	err  error
}

func (s stubProvider) AuthCodeURL(state, challenge string) string {
	return "https://accounts.google.com/o/oauth2/auth?state=" + state + "&code_challenge=" + challenge
}
func (s stubProvider) Exchange(context.Context, string, string) (auth.OAuthUser, error) {
	return s.user, s.err
}

type fakeStates struct{ m map[string]auth.OAuthState }

func (f *fakeStates) Save(_ context.Context, state string, v auth.OAuthState, _ time.Duration) error {
	if f.m == nil {
		f.m = map[string]auth.OAuthState{}
	}
	f.m[state] = v
	return nil
}
func (f *fakeStates) Take(_ context.Context, state string) (auth.OAuthState, error) {
	v, ok := f.m[state]
	if !ok {
		return auth.OAuthState{}, domain.ErrNotFound
	}
	delete(f.m, state)
	return v, nil
}

func testCipher(t *testing.T) *aesgcm.Cipher {
	t.Helper()
	key := make([]byte, aesgcm.KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := aesgcm.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testSigner(t *testing.T) (*jwtx.Signer, *jwtx.Verifier) {
	t.Helper()
	pem, _ := jwtx.GenerateES256PEM()
	priv, _ := jwtx.LoadPrivateKeyPEM(pem)
	signer, err := jwtx.NewSigner(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer, jwtx.VerifierFromSigners(signer)
}

// --- rate limit ------------------------------------------------------------

func TestLoginRateLimited(t *testing.T) {
	svc, users := newTestService(t, &fakeSessions{}, &fakeObs{})
	seedUser(t, users, "a@b.com", "a-strong-passphrase-1")
	// Swap in a denying limiter.
	svc.limiter = &fakeLimiter{deny: true}

	_, err := svc.Login(context.Background(), LoginInput{Email: "a@b.com", Password: "a-strong-passphrase-1"})
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("want RateLimitError, got %v", err)
	}
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatal("RateLimitError should wrap ErrRateLimited")
	}
}

func TestLoginSuccessResetsLimiter(t *testing.T) {
	lim := &fakeLimiter{}
	sess := &fakeSessions{}
	svc, users := newTestService(t, sess, &fakeObs{})
	svc.limiter = lim
	seedUser(t, users, "a@b.com", "a-strong-passphrase-1")

	if _, err := svc.Login(context.Background(), LoginInput{Email: "a@b.com", Password: "a-strong-passphrase-1"}); err != nil {
		t.Fatal(err)
	}
	if lim.resets != 1 {
		t.Fatalf("expected limiter reset on success, got %d", lim.resets)
	}
}

// --- 2FA -------------------------------------------------------------------

func full2FAService(t *testing.T) (*Service, *fakeUsers, *fakeSessions) {
	t.Helper()
	signer, verifier := testSigner(t)
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	sess := &fakeSessions{}
	svc := New(Deps{
		Users: users, Sessions: sess, Tokens: &fakeOTT{},
		Recovery: &fakeRecovery{}, Cache: &fakeCache{active: map[string]bool{}},
		Signer: signer, Verifier: verifier, Cipher: testCipher(t),
	})
	return svc, users, sess
}

func TestEnrollActivateThenLoginRequires2FA(t *testing.T) {
	svc, users, _ := full2FAService(t)
	seedUser(t, users, "a@b.com", "a-strong-passphrase-1")

	enr, err := svc.Enroll2FA(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if enr.Secret == "" || enr.ProvisioningURI == "" {
		t.Fatal("expected secret + provisioning uri")
	}
	// Not yet enabled until activate.
	code, _ := totp.Code(enr.Secret, time.Now())
	codes, err := svc.Activate2FA(context.Background(), "user-1", code)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if len(codes) != recoveryCodeCount {
		t.Fatalf("expected %d recovery codes, got %d", recoveryCodeCount, len(codes))
	}

	// Login now returns a pending-2FA challenge, not tokens.
	res, err := svc.Login(context.Background(), LoginInput{Email: "a@b.com", Password: "a-strong-passphrase-1"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !res.TwoFactor || res.PendingToken == "" || res.Tokens != nil {
		t.Fatalf("expected 2FA challenge, got %+v", res)
	}

	// Exchange the pending token + a fresh code for a session.
	code2, _ := totp.Code(enr.Secret, time.Now())
	tokens, err := svc.Verify2FA(context.Background(), res.PendingToken, code2, "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("verify2fa: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("expected access token after 2FA")
	}
}

func TestVerify2FARejectsBadCode(t *testing.T) {
	svc, users, _ := full2FAService(t)
	seedUser(t, users, "a@b.com", "a-strong-passphrase-1")
	enr, _ := svc.Enroll2FA(context.Background(), "user-1")
	code, _ := totp.Code(enr.Secret, time.Now())
	_, _ = svc.Activate2FA(context.Background(), "user-1", code)

	res, _ := svc.Login(context.Background(), LoginInput{Email: "a@b.com", Password: "a-strong-passphrase-1"})
	if _, err := svc.Verify2FA(context.Background(), res.PendingToken, "000000", "ua", "ip"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestPendingTokenIsNotAnAccessToken(t *testing.T) {
	svc, users, _ := full2FAService(t)
	seedUser(t, users, "a@b.com", "a-strong-passphrase-1")
	enr, _ := svc.Enroll2FA(context.Background(), "user-1")
	code, _ := totp.Code(enr.Secret, time.Now())
	_, _ = svc.Activate2FA(context.Background(), "user-1", code)
	res, _ := svc.Login(context.Background(), LoginInput{Email: "a@b.com", Password: "a-strong-passphrase-1"})

	// The pending token must be rejected by the access-token verifier (scope guard).
	if _, err := svc.verifier.Verify(res.PendingToken); err == nil {
		t.Fatal("pending token must not verify as an access token")
	}
}

// --- password reset / change ----------------------------------------------

func TestResetPasswordRevokesSessions(t *testing.T) {
	signer, _ := testSigner(t)
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	seedUser(t, users, "a@b.com", "old-strong-passphrase-1")
	ott := &fakeOTT{consume: &auth.OneTimeToken{UserID: "user-1", Purpose: auth.PurposePasswordReset}}
	sess := &fakeSessions{}
	svc := New(Deps{Users: users, Sessions: sess, Tokens: ott,
		Cache: &fakeCache{active: map[string]bool{}}, Signer: signer})

	if err := svc.ResetPassword(context.Background(), "raw-token", "new-strong-passphrase-9"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !ott.consumed {
		t.Fatal("reset token should be consumed")
	}
	if sess.revokedAll != 1 {
		t.Fatalf("reset must revoke all sessions, got %d", sess.revokedAll)
	}
}

func TestChangePasswordVerifiesCurrentAndRevokes(t *testing.T) {
	signer, _ := testSigner(t)
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	seedUser(t, users, "a@b.com", "old-strong-passphrase-1")
	sess := &fakeSessions{}
	svc := New(Deps{Users: users, Sessions: sess, Tokens: &fakeOTT{},
		Cache: &fakeCache{active: map[string]bool{}}, Signer: signer})

	// Wrong current password → 401.
	if err := svc.ChangePassword(context.Background(), "user-1", "wrong", "new-strong-passphrase-9", "sid"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	// Correct current password → succeeds and revokes.
	if err := svc.ChangePassword(context.Background(), "user-1", "old-strong-passphrase-1", "new-strong-passphrase-9", "sid"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if sess.revokedAll != 1 {
		t.Fatalf("change must revoke all sessions, got %d", sess.revokedAll)
	}
}

func TestForgotPasswordSilentForUnknownEmail(t *testing.T) {
	signer, _ := testSigner(t)
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	mail := &fakeMailer{}
	svc := New(Deps{Users: users, Sessions: &fakeSessions{}, Tokens: &fakeOTT{},
		Cache: &fakeCache{active: map[string]bool{}}, Signer: signer, Mailer: mail})

	if err := svc.ForgotPassword(context.Background(), "nobody@b.com"); err != nil {
		t.Fatalf("forgot: %v", err)
	}
	if mail.reset != 0 {
		t.Fatal("no email should be sent for unknown address")
	}
}

// --- OAuth linking ---------------------------------------------------------

func oauthService(t *testing.T, prov auth.OAuthProvider) (*Service, *fakeUsers, *fakeOAuth) {
	t.Helper()
	signer, _ := testSigner(t)
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	oa := &fakeOAuth{bySub: map[string]*auth.OAuthIdentity{}}
	svc := New(Deps{Users: users, Sessions: &fakeSessions{}, Tokens: &fakeOTT{}, OAuth: oa,
		Cache: &fakeCache{active: map[string]bool{}}, Signer: signer, Google: prov})
	return svc, users, oa
}

type fakeOAuth struct {
	bySub   map[string]*auth.OAuthIdentity
	created int
}

func (f *fakeOAuth) GetByProviderSub(_ context.Context, _, sub string) (*auth.OAuthIdentity, error) {
	if id, ok := f.bySub[sub]; ok {
		return id, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeOAuth) Create(_ context.Context, oi *auth.OAuthIdentity) error {
	f.created++
	f.bySub[oi.ProviderSub] = oi
	return nil
}

func TestOAuthNewUserIsCreatedVerified(t *testing.T) {
	prov := stubProvider{user: auth.OAuthUser{Sub: "g-1", Email: "new@b.com", EmailVerified: true, Name: "New"}}
	svc, users, oa := oauthService(t, prov)
	states := &fakeStates{}

	url, err := svc.StartOAuth(context.Background(), states, "/dashboard")
	if err != nil || url == "" {
		t.Fatalf("start: %v url=%q", err, url)
	}
	var state string
	for s := range states.m {
		state = s
	}
	res, err := svc.CompleteOAuth(context.Background(), states, "code", state, "ua", "ip")
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if res.Tokens.AccessToken == "" || res.RedirectAfter != "/dashboard" {
		t.Fatalf("unexpected result %+v", res)
	}
	u := users.byEmail["new@b.com"]
	if u == nil || !u.EmailVerified {
		t.Fatal("expected a new verified user")
	}
	if oa.created != 1 {
		t.Fatalf("expected identity linked, got %d", oa.created)
	}
}

func TestOAuthUnverifiedLocalUserRefused(t *testing.T) {
	prov := stubProvider{user: auth.OAuthUser{Sub: "g-2", Email: "a@b.com", EmailVerified: true}}
	svc, users, _ := oauthService(t, prov)
	users.byEmail["a@b.com"] = &auth.User{ID: "user-1", Email: "a@b.com", EmailVerified: false}
	states := &fakeStates{}
	_, _ = svc.StartOAuth(context.Background(), states, "/")
	var state string
	for s := range states.m {
		state = s
	}
	_, err := svc.CompleteOAuth(context.Background(), states, "code", state, "ua", "ip")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("want ErrConflict (verify_first), got %v", err)
	}
}

func TestOAuthStateSingleUse(t *testing.T) {
	prov := stubProvider{user: auth.OAuthUser{Sub: "g-3", Email: "c@b.com", EmailVerified: true}}
	svc, _, _ := oauthService(t, prov)
	states := &fakeStates{}
	_, _ = svc.StartOAuth(context.Background(), states, "/")
	var state string
	for s := range states.m {
		state = s
	}
	if _, err := svc.CompleteOAuth(context.Background(), states, "code", state, "ua", "ip"); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	// Replaying the same state must fail (deleted on read).
	if _, err := svc.CompleteOAuth(context.Background(), states, "code", state, "ua", "ip"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized on replay, got %v", err)
	}
}
