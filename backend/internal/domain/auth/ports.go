package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
)

// ErrRefreshReuse is returned when a rotated refresh token is presented again —
// evidence of theft. It wraps domain.ErrUnauthorized so the HTTP layer still
// maps it to 401, while the usecase can detect the reuse (errors.Is) to revoke
// the family and emit a security audit entry (docs/04-AUTH.md §3).
var ErrRefreshReuse = fmt.Errorf("refresh token reuse detected: %w", domain.ErrUnauthorized)

// UserRepository persists global user accounts (no tenant scope).
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	// GetByEmail returns domain.ErrNotFound if no such user.
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByID returns domain.ErrNotFound if no such user.
	GetByID(ctx context.Context, id string) (*User, error)
	UpdatePasswordHash(ctx context.Context, id, hash string) error
	MarkEmailVerified(ctx context.Context, id string) error
	// SetTOTP stores the (encrypted) TOTP secret and enabled flag together, or
	// clears both when encSecret is empty (docs/04-AUTH.md §4).
	SetTOTP(ctx context.Context, id, encSecret string, enabled bool) error
}

// RecoveryCodeRepository persists single-use 2FA recovery codes (stored hashed).
type RecoveryCodeRepository interface {
	// Replace atomically swaps the user's codes for a fresh batch (used on
	// activate / regenerate).
	Replace(ctx context.Context, userID string, codeHashes [][]byte) error
	// Consume marks one matching unused code used and reports whether it hit.
	Consume(ctx context.Context, userID string, codeHash []byte) (consumed bool, err error)
}

// OAuthIdentityRepository links external provider identities to users (Google,
// docs/04-AUTH.md §4).
type OAuthIdentityRepository interface {
	// GetByProviderSub returns domain.ErrNotFound when the identity is unknown.
	GetByProviderSub(ctx context.Context, provider, sub string) (*OAuthIdentity, error)
	Create(ctx context.Context, oi *OAuthIdentity) error
}

// OAuthUser is the normalized identity a provider returns after a code exchange.
type OAuthUser struct {
	Sub           string // stable provider subject id
	Email         string
	EmailVerified bool
	Name          string
}

// OAuthProvider abstracts an external IdP (Google) so the usecase stays off the
// golang.org/x/oauth2 dependency. Implemented in cmd/api.
type OAuthProvider interface {
	// AuthCodeURL builds the consent URL for the given state and PKCE S256
	// challenge.
	AuthCodeURL(state, codeChallenge string) string
	// Exchange swaps an authorization code + PKCE verifier for the user profile.
	Exchange(ctx context.Context, code, codeVerifier string) (OAuthUser, error)
}

// OAuthState is the per-flow data stashed under an opaque state token during a
// PKCE login (docs/04-AUTH.md §4).
type OAuthState struct {
	Verifier      string `json:"verifier"`
	RedirectAfter string `json:"redirect_after"`
}

// OAuthStateStore persists OAuthState for the duration of a flow. Implemented in
// infrastructure/redis.
type OAuthStateStore interface {
	Save(ctx context.Context, state string, v OAuthState, ttl time.Duration) error
	// Take returns and deletes the state (single use); domain.ErrNotFound when
	// absent/expired.
	Take(ctx context.Context, state string) (OAuthState, error)
}

// SessionRepository persists refresh-token sessions and owns the transactional
// rotation (docs/04-AUTH.md §3). Rotation logic lives here because it must run
// in one serializable transaction with a row lock.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	// GetByID returns domain.ErrNotFound if no such session (used for cache
	// backfill in the Auth middleware).
	GetByID(ctx context.Context, id string) (*Session, error)

	// Rotate atomically exchanges the refresh token identified by presentedHash:
	// it locks the row, and
	//   - not found / revoked / expired -> domain.ErrUnauthorized
	//   - already rotated                -> revokes the whole family, ErrRefreshReuse
	//   - otherwise                      -> sets rotated_at, inserts a new session
	//     (same FamilyID, new id/hash/expiry) and returns it.
	Rotate(ctx context.Context, presentedHash, newHash []byte, newID, userAgent, ip string, newExpiry time.Time) (*Session, error)

	// RevokeByID revokes a single session (logout).
	RevokeByID(ctx context.Context, id, reason string) error
	// RevokeAllForUser revokes every active session of a user (logout-all,
	// password change/reset — docs/04-AUTH.md §6).
	RevokeAllForUser(ctx context.Context, userID, reason string) error

	// ListForUser returns the user's live sessions (the current head of each
	// refresh family), newest first, for GET /auth/sessions.
	ListForUser(ctx context.Context, userID string) ([]*Session, error)
	// RevokeByIDForUser revokes a single session owned by userID (ownership
	// scoped so one user cannot revoke another's). domain.ErrNotFound when no
	// matching active session exists.
	RevokeByIDForUser(ctx context.Context, id, userID, reason string) error
}

// OneTimeTokenRepository persists single-use tokens (email verify / reset).
type OneTimeTokenRepository interface {
	Create(ctx context.Context, t *OneTimeToken) error
	// Consume atomically marks the token used (UPDATE ... WHERE used_at IS NULL
	// RETURNING) and returns it; domain.ErrNotFound if missing/expired/used.
	Consume(ctx context.Context, purpose TokenPurpose, hash []byte, now time.Time) (*OneTimeToken, error)
}

// LoginRateLimiter throttles password-login attempts keyed by (email, IP) to
// blunt credential stuffing (docs/04-AUTH.md §4, FR-AUTH-011). Implemented in
// infrastructure/redis over a fixed window.
type LoginRateLimiter interface {
	// Allow records one attempt against key and reports whether it stays within
	// budget. When denied, retryAfter is a suggested backoff (window remaining).
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
	// Reset clears the counter for key (called after a successful credential
	// check so a legitimate user is never locked out by their own earlier typos).
	Reset(ctx context.Context, key string) error
}

// SessionCache bounds revocation latency: the Auth middleware consults it before
// touching Postgres (docs/04-AUTH.md §2). Implemented in infrastructure/redis.
type SessionCache interface {
	// MarkActive records that sid is a live session for ttl.
	MarkActive(ctx context.Context, sid string, ttl time.Duration) error
	// Status returns (active, known). known=false means a cache miss (backfill
	// from the SessionRepository).
	Status(ctx context.Context, sid string) (active, known bool, err error)
	// Revoke marks sid revoked (or removes it) so subsequent requests 401.
	Revoke(ctx context.Context, sid string) error
}
