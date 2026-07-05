// Package authuc holds the authentication application services: register,
// login, refresh-rotation, and logout (docs/04-AUTH.md). It depends only on
// domain interfaces and leaf pkgs (argon2x, jwtx, token, clock, uuidv7) — never
// on infrastructure; wiring happens in cmd/api.
package authuc

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/argon2x"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/clock"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// sessionCacheTTL is how long a session's "active" status is trusted without a
// Postgres re-check (ADR 008). Revocation latency is bounded by this.
const sessionCacheTTL = 60 * time.Second

// emailVerifyTTL matches docs/04-AUTH.md §2 (24h).
const emailVerifyTTL = 24 * time.Hour

// passwordResetTTL matches docs/04-AUTH.md §2 (1h).
const passwordResetTTL = time.Hour

// pending2FATTL matches docs/04-AUTH.md §2: the pending-2FA JWT lives 5 min.
const pending2FATTL = 5 * time.Minute

// Observer receives auth metrics/security signals (docs/10-INFRA-DEVOPS.md §5).
// Implemented in cmd/api against Prometheus counters + audit.
type Observer interface {
	LoginAttempt(ctx context.Context, result string) // "success" | "invalid" | "locked"
	RefreshReuse(ctx context.Context)
}

// Service implements the auth usecases.
type Service struct {
	users    auth.UserRepository
	sessions auth.SessionRepository
	tokens   auth.OneTimeTokenRepository
	recovery auth.RecoveryCodeRepository
	oauth    auth.OAuthIdentityRepository
	cache    auth.SessionCache
	limiter  auth.LoginRateLimiter
	mailer   Mailer
	signer   *jwtx.Signer
	verifier *jwtx.Verifier
	cipher   SecretCipher
	google   auth.OAuthProvider
	clock    clock.Clock
	obs      Observer
	auditor  audit.Writer

	accessTTL  time.Duration
	refreshTTL time.Duration
}

// Deps bundles Service collaborators. Repos/collaborators used only by specific
// slices (recovery, oauth, cipher, google, verifier) may be nil in tests that do
// not exercise those flows; the corresponding methods then return an error.
type Deps struct {
	Users    auth.UserRepository
	Sessions auth.SessionRepository
	Tokens   auth.OneTimeTokenRepository
	Recovery auth.RecoveryCodeRepository
	OAuth    auth.OAuthIdentityRepository
	Cache    auth.SessionCache
	Limiter  auth.LoginRateLimiter
	Mailer   Mailer
	Signer   *jwtx.Signer
	Verifier *jwtx.Verifier
	Cipher   SecretCipher
	Google   auth.OAuthProvider
	Clock    clock.Clock
	Observer Observer
	Audit    audit.Writer

	AccessTTL  time.Duration // default 15m
	RefreshTTL time.Duration // default 7d
}

// New builds a Service, applying default TTLs and no-op fallbacks for optional
// collaborators (limiter/mailer) so tests can omit them.
func New(d Deps) *Service {
	if d.AccessTTL == 0 {
		d.AccessTTL = 15 * time.Minute
	}
	if d.RefreshTTL == 0 {
		d.RefreshTTL = 7 * 24 * time.Hour
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Limiter == nil {
		d.Limiter = noopLimiter{}
	}
	if d.Mailer == nil {
		d.Mailer = noopMailer{}
	}
	if d.Audit == nil {
		d.Audit = noopAudit{}
	}
	return &Service{
		users: d.Users, sessions: d.Sessions, tokens: d.Tokens,
		recovery: d.Recovery, oauth: d.OAuth, cache: d.Cache,
		limiter: d.Limiter, mailer: d.Mailer,
		signer: d.Signer, verifier: d.Verifier, cipher: d.Cipher, google: d.Google,
		clock: d.Clock, obs: d.Observer, auditor: d.Audit,
		accessTTL: d.AccessTTL, refreshTTL: d.RefreshTTL,
	}
}

// writeAudit appends an audit entry best-effort, enriching missing actor/IP/UA
// from the request context (pkg/reqmeta). A failed write is logged, never
// surfaced — audit is telemetry, not part of the request contract.
func (s *Service) writeAudit(ctx context.Context, e audit.Entry) {
	m := reqmeta.From(ctx)
	if e.ActorUserID == "" {
		e.ActorUserID = m.ActorUserID
	}
	if e.IP == "" {
		e.IP = m.IP
	}
	if e.UserAgent == "" {
		e.UserAgent = m.UserAgent
	}
	if err := s.auditor.Append(ctx, e); err != nil {
		slog.Default().WarnContext(ctx, "audit append failed", "action", e.Action, "err", err)
	}
}

type noopAudit struct{}

func (noopAudit) Append(context.Context, audit.Entry) error { return nil }

// SecretCipher encrypts/decrypts secrets at rest (the TOTP secret). Satisfied by
// pkg/aesgcm.Cipher; an interface here keeps the usecase off infrastructure.
type SecretCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Tokens is the result of a successful login/refresh. RefreshToken is the raw
// secret to place in the httpOnly cookie; only its hash is stored.
type Tokens struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionID        string
	UserID           string
}

// --- Register -------------------------------------------------------------

// RegisterInput carries the registration form.
type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

// RegisterResult is intentionally non-committal: the caller returns the same
// generic "check your email" response whether or not the account was created,
// to avoid user enumeration (docs/04-AUTH.md §6). VerifyToken is the raw
// email-verification token (empty when nothing was created).
type RegisterResult struct {
	Created     bool
	UserID      string
	VerifyToken string
}

// Register validates the password policy, creates an unverified user, and mints
// an email-verification token. A duplicate email is swallowed into a
// non-created result (no enumeration).
func (s *Service) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	email := normalizeEmail(in.Email)
	if !validEmail(email) {
		return RegisterResult{}, fmt.Errorf("email: %w", domain.ErrValidation)
	}
	if err := argon2x.CheckPolicy(in.Password, email); err != nil {
		return RegisterResult{}, fmt.Errorf("%s: %w", err.Error(), domain.ErrValidation)
	}

	hash, err := argon2x.Hash(in.Password)
	if err != nil {
		return RegisterResult{}, err
	}

	now := s.clock.Now()
	u := &auth.User{
		ID:           uuidv7.New().String(),
		Email:        email,
		PasswordHash: hash,
		Name:         strings.TrimSpace(in.Name),
		PlatformRole: auth.PlatformRoleUser,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return RegisterResult{Created: false}, nil // email taken -> generic response
		}
		return RegisterResult{}, err
	}

	rawVerify, err := s.mintEmailVerify(ctx, u.ID, now)
	if err != nil {
		return RegisterResult{}, err
	}
	if err := s.mailer.SendEmailVerify(ctx, u.Email, rawVerify); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Created: true, UserID: u.ID, VerifyToken: rawVerify}, nil
}

func (s *Service) mintEmailVerify(ctx context.Context, userID string, now time.Time) (string, error) {
	raw, err := token.New()
	if err != nil {
		return "", err
	}
	ott := &auth.OneTimeToken{
		ID:        uuidv7.New().String(),
		Purpose:   auth.PurposeEmailVerify,
		UserID:    userID,
		TokenHash: token.Hash(raw),
		ExpiresAt: now.Add(emailVerifyTTL),
	}
	if err := s.tokens.Create(ctx, ott); err != nil {
		return "", err
	}
	return raw, nil
}

// ResendEmailVerify re-mints and re-sends the verification link. To avoid
// enumeration it returns nil whether the email is unknown or already verified
// (docs/04-AUTH.md §6).
func (s *Service) ResendEmailVerify(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if u.EmailVerified {
		return nil
	}
	raw, err := s.mintEmailVerify(ctx, u.ID, s.clock.Now())
	if err != nil {
		return err
	}
	return s.mailer.SendEmailVerify(ctx, u.Email, raw)
}

// VerifyEmail consumes an email-verification token and marks the user verified.
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	ott, err := s.tokens.Consume(ctx, auth.PurposeEmailVerify, token.Hash(rawToken), s.clock.Now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrUnauthorized // invalid/expired/used
		}
		return err
	}
	return s.users.MarkEmailVerified(ctx, ott.UserID)
}

// --- Password reset / change ----------------------------------------------

// ForgotPassword mints a 1h password-reset token and emails it. To avoid
// enumeration it returns nil whether or not the email exists (docs/04-AUTH.md §6);
// an unknown address simply does no work.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil // silent: no enumeration
		}
		return err
	}
	// OAuth-only accounts have no password to reset; stay silent all the same.
	if !u.HasPassword() {
		return nil
	}
	raw, err := token.New()
	if err != nil {
		return err
	}
	now := s.clock.Now()
	ott := &auth.OneTimeToken{
		ID:        uuidv7.New().String(),
		Purpose:   auth.PurposePasswordReset,
		UserID:    u.ID,
		TokenHash: token.Hash(raw),
		ExpiresAt: now.Add(passwordResetTTL),
	}
	if err := s.tokens.Create(ctx, ott); err != nil {
		return err
	}
	return s.mailer.SendPasswordReset(ctx, u.Email, raw)
}

// ResetPassword consumes a password-reset token, sets the new password (policy
// enforced), and revokes every session so a leaked token cannot outlive the
// reset (FR-AUTH-010/012, docs/04-AUTH.md §6).
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	ott, err := s.tokens.Consume(ctx, auth.PurposePasswordReset, token.Hash(rawToken), s.clock.Now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrUnauthorized // invalid / expired / used
		}
		return err
	}
	u, err := s.users.GetByID(ctx, ott.UserID)
	if err != nil {
		return err
	}
	if err := argon2x.CheckPolicy(newPassword, u.Email); err != nil {
		return fmt.Errorf("%s: %w", err.Error(), domain.ErrValidation)
	}
	hash, err := argon2x.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePasswordHash(ctx, u.ID, hash); err != nil {
		return err
	}
	if err := s.sessions.RevokeAllForUser(ctx, u.ID, "password_reset"); err != nil {
		return err
	}
	s.writeAudit(ctx, audit.Entry{
		ActorUserID: u.ID,
		Action:      audit.ActionPasswordReset,
		Severity:    audit.SeveritySecurity,
	})
	return nil
}

// ChangePassword verifies the current password, sets a new one, and revokes all
// sessions (including the caller's) so every device must re-authenticate — the
// secure default for a credential change (FR-AUTH-010). The caller's cache entry
// is evicted immediately so the change takes effect without waiting for the TTL.
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentSID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.HasPassword() {
		return domain.ErrForbidden // OAuth-only account: nothing to change here
	}
	ok, err := argon2x.Verify(currentPassword, u.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrUnauthorized
	}
	if err := argon2x.CheckPolicy(newPassword, u.Email); err != nil {
		return fmt.Errorf("%s: %w", err.Error(), domain.ErrValidation)
	}
	hash, err := argon2x.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePasswordHash(ctx, u.ID, hash); err != nil {
		return err
	}
	if err := s.sessions.RevokeAllForUser(ctx, u.ID, "password_change"); err != nil {
		return err
	}
	if currentSID != "" {
		_ = s.cache.Revoke(ctx, currentSID)
	}
	s.writeAudit(ctx, audit.Entry{
		ActorUserID: u.ID,
		Action:      audit.ActionPasswordChange,
		Severity:    audit.SeveritySecurity,
	})
	return nil
}

// --- Login ----------------------------------------------------------------

// LoginInput carries credentials plus request metadata for the session row.
type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

// LoginResult is the outcome of a password login. Exactly one of Tokens (login
// complete) or TwoFactor (a second factor is required) is set. When TwoFactor
// is true, PendingToken is a short-lived JWT (scope=pending_2fa) the client
// echoes back to POST /auth/2fa/verify.
type LoginResult struct {
	Tokens       *Tokens
	TwoFactor    bool
	PendingToken string
}

// RateLimitError signals the login throttle tripped (docs/04-AUTH.md §4). It
// wraps domain.ErrRateLimited (→ 429) and carries the suggested backoff so the
// HTTP layer can set Retry-After.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string { return "too many login attempts" }
func (e *RateLimitError) Unwrap() error { return domain.ErrRateLimited }

// Login gates on the per-(email,IP) rate limiter, verifies credentials with
// uniform timing, and either issues a session or (TOTP users) returns a pending
// 2FA token. All credential failures return the same domain.ErrUnauthorized so
// nothing leaks account existence (no enumeration).
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	email := normalizeEmail(in.Email)

	rlKey := loginRLKey(email, in.IP)
	allowed, retryAfter, err := s.limiter.Allow(ctx, rlKey)
	if err != nil {
		return LoginResult{}, err
	}
	if !allowed {
		s.observe(ctx).LoginAttempt(ctx, "rate_limited")
		return LoginResult{}, &RateLimitError{RetryAfter: retryAfter}
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Spend the same time as a real verify so timing does not leak
			// account existence (docs/04-AUTH.md §4).
			_, _ = argon2x.Verify(in.Password, argon2x.DummyHash)
			s.observe(ctx).LoginAttempt(ctx, "invalid")
			return LoginResult{}, domain.ErrUnauthorized
		}
		return LoginResult{}, err
	}

	if u.Locked() {
		s.observe(ctx).LoginAttempt(ctx, "locked")
		return LoginResult{}, domain.ErrUnauthorized
	}
	if !u.HasPassword() {
		_, _ = argon2x.Verify(in.Password, argon2x.DummyHash)
		s.observe(ctx).LoginAttempt(ctx, "invalid")
		return LoginResult{}, domain.ErrUnauthorized
	}

	ok, err := argon2x.Verify(in.Password, u.PasswordHash)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		s.observe(ctx).LoginAttempt(ctx, "invalid")
		s.writeAudit(ctx, audit.Entry{
			ActorUserID: u.ID,
			Action:      audit.ActionLoginFailed,
			Metadata:    map[string]any{"reason": "invalid_password"},
			Severity:    audit.SeverityWarning,
		})
		return LoginResult{}, domain.ErrUnauthorized
	}

	// Credentials are correct: clear the throttle so the user's own earlier
	// typos never lock them out.
	_ = s.limiter.Reset(ctx, rlKey)

	if u.TOTPEnabled {
		pending, err := s.signer.SignPending2FA(u.ID, s.clock.Now(), pending2FATTL)
		if err != nil {
			return LoginResult{}, err
		}
		s.observe(ctx).LoginAttempt(ctx, "2fa_required")
		return LoginResult{TwoFactor: true, PendingToken: pending}, nil
	}

	tokens, err := s.issueSession(ctx, u.ID, in.UserAgent, in.IP)
	if err != nil {
		return LoginResult{}, err
	}
	s.observe(ctx).LoginAttempt(ctx, "success")
	return LoginResult{Tokens: &tokens}, nil
}

// loginRLKey is the throttle key from docs/04-AUTH.md §4:
// rl:login:{sha1(email)}:{ip}. Hashing the email keeps addresses out of Redis.
func loginRLKey(email, ip string) string {
	sum := sha1.Sum([]byte(email))
	return "rl:login:" + hex.EncodeToString(sum[:]) + ":" + ip
}

// issueSession creates a fresh session family and mints access+refresh tokens.
func (s *Service) issueSession(ctx context.Context, userID, userAgent, ip string) (Tokens, error) {
	now := s.clock.Now()
	rawRefresh, err := token.New()
	if err != nil {
		return Tokens{}, err
	}
	sid := uuidv7.New().String()
	sess := &auth.Session{
		ID:        sid,
		FamilyID:  uuidv7.New().String(),
		UserID:    userID,
		TokenHash: token.Hash(rawRefresh),
		UserAgent: userAgent,
		IP:        ip,
		ExpiresAt: now.Add(s.refreshTTL),
		CreatedAt: now,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return Tokens{}, err
	}
	return s.finishTokens(ctx, userID, sid, rawRefresh, now, sess.ExpiresAt)
}

// finishTokens signs the access JWT, primes the session cache, and assembles
// the Tokens result shared by login and refresh.
func (s *Service) finishTokens(ctx context.Context, userID, sid, rawRefresh string, now, refreshExpiry time.Time) (Tokens, error) {
	// Stamp the email-verified flag into the access token so RequireVerified is a
	// claim check, not a per-request DB read. A lookup failure is fail-closed
	// (verified=false → gated), which is the safe default.
	verified := false
	if u, err := s.users.GetByID(ctx, userID); err == nil {
		verified = u.EmailVerified
	}
	access, err := s.signer.Sign(userID, sid, verified, now, s.accessTTL)
	if err != nil {
		return Tokens{}, err
	}
	if err := s.cache.MarkActive(ctx, sid, sessionCacheTTL); err != nil {
		return Tokens{}, err
	}
	return Tokens{
		AccessToken:      access,
		AccessExpiresAt:  now.Add(s.accessTTL),
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExpiry,
		SessionID:        sid,
		UserID:           userID,
	}, nil
}

// --- Refresh --------------------------------------------------------------

// Refresh rotates the presented refresh token (docs/04-AUTH.md §3). Reuse of a
// rotated token revokes the family and returns auth.ErrRefreshReuse (401).
func (s *Service) Refresh(ctx context.Context, rawRefresh, userAgent, ip string) (Tokens, error) {
	if rawRefresh == "" {
		return Tokens{}, domain.ErrUnauthorized
	}
	now := s.clock.Now()
	rawNew, err := token.New()
	if err != nil {
		return Tokens{}, err
	}
	newID := uuidv7.New().String()

	sess, err := s.sessions.Rotate(
		ctx,
		token.Hash(rawRefresh),
		token.Hash(rawNew),
		newID, userAgent, ip,
		now.Add(s.refreshTTL),
	)
	if err != nil {
		if errors.Is(err, auth.ErrRefreshReuse) {
			s.observe(ctx).RefreshReuse(ctx)
			s.writeAudit(ctx, audit.Entry{
				Action:   audit.ActionRefreshReuse,
				Severity: audit.SeveritySecurity,
			})
		}
		return Tokens{}, err
	}
	return s.finishTokens(ctx, sess.UserID, sess.ID, rawNew, now, sess.ExpiresAt)
}

// --- Logout ---------------------------------------------------------------

// Logout revokes the current session (by sid from the access token) and evicts
// it from the cache immediately.
func (s *Service) Logout(ctx context.Context, sid string) error {
	if sid == "" {
		return nil
	}
	if err := s.sessions.RevokeByID(ctx, sid, "logout"); err != nil {
		return err
	}
	return s.cache.Revoke(ctx, sid)
}

// LogoutAll revokes every session of the user (docs/04-AUTH.md §6). Cache
// entries for other sessions expire within sessionCacheTTL.
func (s *Service) LogoutAll(ctx context.Context, userID, currentSID string) error {
	if err := s.sessions.RevokeAllForUser(ctx, userID, "logout_all"); err != nil {
		return err
	}
	if currentSID != "" {
		_ = s.cache.Revoke(ctx, currentSID)
	}
	return nil
}

// --- Sessions management ---------------------------------------------------

// ListSessions returns the caller's live sessions for GET /auth/sessions.
func (s *Service) ListSessions(ctx context.Context, userID string) ([]*auth.Session, error) {
	return s.sessions.ListForUser(ctx, userID)
}

// RevokeSession revokes one of the caller's sessions by id (DELETE
// /auth/sessions/{id}). Ownership is enforced in the repo; a non-owned or
// already-dead id surfaces as domain.ErrNotFound. The cache is evicted so the
// revocation is immediate rather than bounded by the cache TTL.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	if err := s.sessions.RevokeByIDForUser(ctx, sessionID, userID, "revoked_by_user"); err != nil {
		return err
	}
	s.writeAudit(ctx, audit.Entry{
		ActorUserID: userID,
		Action:      audit.ActionSessionRevoke,
		TargetType:  "session",
		TargetID:    sessionID,
		Severity:    audit.SeverityInfo,
	})
	return s.cache.Revoke(ctx, sessionID)
}

// --- helpers --------------------------------------------------------------

// observe returns the Observer or a no-op if none was wired.
func (s *Service) observe(context.Context) Observer {
	if s.obs == nil {
		return noopObserver{}
	}
	return s.obs
}

type noopObserver struct{}

func (noopObserver) LoginAttempt(context.Context, string) {}
func (noopObserver) RefreshReuse(context.Context)         {}

// Mailer delivers the transactional auth emails. Implemented in
// infrastructure/mailer; a dev build logs instead of sending (docs/04-AUTH.md).
type Mailer interface {
	SendEmailVerify(ctx context.Context, to, rawToken string) error
	SendPasswordReset(ctx context.Context, to, rawToken string) error
}

type noopMailer struct{}

func (noopMailer) SendEmailVerify(context.Context, string, string) error   { return nil }
func (noopMailer) SendPasswordReset(context.Context, string, string) error { return nil }

type noopLimiter struct{}

func (noopLimiter) Allow(context.Context, string) (bool, time.Duration, error) {
	return true, 0, nil
}
func (noopLimiter) Reset(context.Context, string) error { return nil }

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validEmail(e string) bool {
	at := strings.IndexByte(e, '@')
	return at > 0 && at < len(e)-1 && !strings.ContainsAny(e, " \t\n")
}
