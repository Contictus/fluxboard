// Package authuc holds the authentication application services: register,
// login, refresh-rotation, and logout (docs/04-AUTH.md). It depends only on
// domain interfaces and leaf pkgs (argon2x, jwtx, token, clock, uuidv7) — never
// on infrastructure; wiring happens in cmd/api.
package authuc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/argon2x"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/clock"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// sessionCacheTTL is how long a session's "active" status is trusted without a
// Postgres re-check (ADR 008). Revocation latency is bounded by this.
const sessionCacheTTL = 60 * time.Second

// emailVerifyTTL matches docs/04-AUTH.md §2 (24h).
const emailVerifyTTL = 24 * time.Hour

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
	cache    auth.SessionCache
	signer   *jwtx.Signer
	clock    clock.Clock
	obs      Observer

	accessTTL  time.Duration
	refreshTTL time.Duration
}

// Deps bundles Service collaborators.
type Deps struct {
	Users    auth.UserRepository
	Sessions auth.SessionRepository
	Tokens   auth.OneTimeTokenRepository
	Cache    auth.SessionCache
	Signer   *jwtx.Signer
	Clock    clock.Clock
	Observer Observer

	AccessTTL  time.Duration // default 15m
	RefreshTTL time.Duration // default 7d
}

// New builds a Service, applying default TTLs.
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
	return &Service{
		users: d.Users, sessions: d.Sessions, tokens: d.Tokens, cache: d.Cache,
		signer: d.Signer, clock: d.Clock, obs: d.Observer,
		accessTTL: d.AccessTTL, refreshTTL: d.RefreshTTL,
	}
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

// --- Login ----------------------------------------------------------------

// LoginInput carries credentials plus request metadata for the session row.
type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

// Login verifies credentials with uniform timing and issues a new session.
// All failure causes return the same domain.ErrUnauthorized (no enumeration).
func (s *Service) Login(ctx context.Context, in LoginInput) (Tokens, error) {
	email := normalizeEmail(in.Email)

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Spend the same time as a real verify so timing does not leak
			// account existence (docs/04-AUTH.md §4).
			_, _ = argon2x.Verify(in.Password, argon2x.DummyHash)
			s.observe(ctx).LoginAttempt(ctx, "invalid")
			return Tokens{}, domain.ErrUnauthorized
		}
		return Tokens{}, err
	}

	if u.Locked() {
		s.observe(ctx).LoginAttempt(ctx, "locked")
		return Tokens{}, domain.ErrUnauthorized
	}
	if !u.HasPassword() {
		_, _ = argon2x.Verify(in.Password, argon2x.DummyHash)
		s.observe(ctx).LoginAttempt(ctx, "invalid")
		return Tokens{}, domain.ErrUnauthorized
	}

	ok, err := argon2x.Verify(in.Password, u.PasswordHash)
	if err != nil {
		return Tokens{}, err
	}
	if !ok {
		s.observe(ctx).LoginAttempt(ctx, "invalid")
		return Tokens{}, domain.ErrUnauthorized
	}

	// TODO(phase1 2FA): if u.TOTPEnabled -> return pending-2fa token instead.

	tokens, err := s.issueSession(ctx, u.ID, in.UserAgent, in.IP)
	if err != nil {
		return Tokens{}, err
	}
	s.observe(ctx).LoginAttempt(ctx, "success")
	return tokens, nil
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
	access, err := s.signer.Sign(userID, sid, now, s.accessTTL)
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

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validEmail(e string) bool {
	at := strings.IndexByte(e, '@')
	return at > 0 && at < len(e)-1 && !strings.ContainsAny(e, " \t\n")
}
