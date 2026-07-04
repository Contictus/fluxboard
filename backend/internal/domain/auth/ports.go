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
}

// OneTimeTokenRepository persists single-use tokens (email verify / reset).
type OneTimeTokenRepository interface {
	Create(ctx context.Context, t *OneTimeToken) error
	// Consume atomically marks the token used (UPDATE ... WHERE used_at IS NULL
	// RETURNING) and returns it; domain.ErrNotFound if missing/expired/used.
	Consume(ctx context.Context, purpose TokenPurpose, hash []byte, now time.Time) (*OneTimeToken, error)
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
