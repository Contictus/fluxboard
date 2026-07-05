// Package auth is the authentication domain: users, sessions (refresh-token
// families), OAuth identities, and one-time tokens, plus the repository/port
// interfaces the usecase layer depends on. Stdlib-only (IDs are strings,
// timestamps time.Time) — no external imports, per docs/CLAUDE.md.
package auth

import "time"

// PlatformRole is a user's platform-wide role (not org-scoped).
type PlatformRole string

const (
	PlatformRoleUser  PlatformRole = "user"
	PlatformRoleAdmin PlatformRole = "admin"
)

// User is a global account. PasswordHash is empty for OAuth-only accounts.
type User struct {
	ID            string
	Email         string
	PasswordHash  string
	Name          string
	AvatarKey     string
	EmailVerified bool
	PlatformRole  PlatformRole
	TOTPSecret    string // encrypted at rest
	TOTPEnabled   bool
	LockedAt      *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// HasPassword reports whether the account can log in with a password.
func (u *User) HasPassword() bool { return u.PasswordHash != "" }

// Locked reports whether the account is currently locked out.
func (u *User) Locked() bool { return u.LockedAt != nil }

// Session is one refresh-token instance; sessions sharing a FamilyID form a
// rotation family (docs/04-AUTH.md §3). The raw refresh token is never stored —
// only its SHA-256 in TokenHash.
type Session struct {
	ID           string
	FamilyID     string
	UserID       string
	TokenHash    []byte
	UserAgent    string
	IP           string
	ExpiresAt    time.Time
	RotatedAt    *time.Time
	RevokedAt    *time.Time
	RevokeReason string
	CreatedAt    time.Time
	LastUsedAt   time.Time
}

// Active reports whether the session can still be exchanged: not rotated, not
// revoked, not expired.
func (s *Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && s.RotatedAt == nil && s.ExpiresAt.After(now)
}

// OAuthIdentity links a user to an external identity provider (Phase 1 Google).
type OAuthIdentity struct {
	ID          string
	UserID      string
	Provider    string
	ProviderSub string
	CreatedAt   time.Time
}

// TokenPurpose enumerates one-time-token uses.
type TokenPurpose string

const (
	PurposeEmailVerify   TokenPurpose = "email_verify"
	PurposePasswordReset TokenPurpose = "password_reset"
	PurposeEmailChange   TokenPurpose = "email_change"
)

// OneTimeToken backs email-verify / password-reset / email-change links. Stored
// hashed and consumed transactionally (single-use).
type OneTimeToken struct {
	ID        string
	Purpose   TokenPurpose
	UserID    string
	TokenHash []byte
	Payload   map[string]any
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}
