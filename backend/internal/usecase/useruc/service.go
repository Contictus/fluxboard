// Package useruc is the account/self-service usecase (docs/08 §3): the caller's
// own profile, avatar, and account deletion. It is user-scoped and org-independent
// — no tenant context — so it does not go through the tenant pool. Ports are the
// narrow interfaces this package needs; infrastructure satisfies them.
package useruc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

const (
	avatarUploadTTL   = 15 * time.Minute
	avatarDownloadTTL = 5 * time.Minute
	maxAvatarBytes    = 5 << 20 // 5 MiB
)

// allowedAvatarTypes bounds the presigned upload to image content types.
var allowedAvatarTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

// UserStore reads and mutates the caller's own user row (global, no RLS).
type UserStore interface {
	GetByID(ctx context.Context, id string) (*auth.User, error)
	UpdateName(ctx context.Context, id, name string) error
	UpdateAvatarKey(ctx context.Context, id, key string) error
	Delete(ctx context.Context, id string) error
}

// AvatarStore issues presigned URLs for the avatar object (MinIO).
type AvatarStore interface {
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error)
}

// Sessions revokes the caller's sessions on account deletion.
type Sessions interface {
	RevokeAllForUser(ctx context.Context, userID, reason string) error
}

// OrgRef is a minimal org reference for the sole-owner block list (JSON body).
type OrgRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// SoleOwnerReader answers which orgs a user solely owns (cross-org, owner pool).
// It returns a domain type so infrastructure need not import this package.
type SoleOwnerReader interface {
	SoleOwnerOrgs(ctx context.Context, userID string) ([]tenant.Organization, error)
}

// SoleOwnerError blocks account deletion, carrying the orgs that must first get a
// new owner (docs/08 §3: 409 sole_owner_of). The HTTP layer renders the list.
type SoleOwnerError struct {
	Orgs []OrgRef
}

func (e *SoleOwnerError) Error() string {
	return fmt.Sprintf("account is sole owner of %d organization(s)", len(e.Orgs))
}

// Profile is the caller's account view (GET /me).
type Profile struct {
	ID            string
	Email         string
	Name          string
	AvatarURL     string // presigned GET, empty when no avatar set
	EmailVerified bool
	PlatformRole  string
	TOTPEnabled   bool
	CreatedAt     time.Time
}

// Deps are the useruc collaborators.
type Deps struct {
	Users     UserStore
	Avatars   AvatarStore
	Sessions  Sessions
	SoleOwner SoleOwnerReader // nil ⇒ account deletion disabled (owner pool absent)
	Logger    *slog.Logger
}

// Service implements the account usecase.
type Service struct {
	users     UserStore
	avatars   AvatarStore
	sessions  Sessions
	soleOwner SoleOwnerReader
	logger    *slog.Logger
}

// New builds the Service.
func New(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{
		users:     d.Users,
		avatars:   d.Avatars,
		sessions:  d.Sessions,
		soleOwner: d.SoleOwner,
		logger:    d.Logger,
	}
}

// Get returns the caller's profile, minting a short-lived avatar URL when set.
func (s *Service) Get(ctx context.Context, userID string) (*Profile, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	p := &Profile{
		ID: u.ID, Email: u.Email, Name: u.Name,
		EmailVerified: u.EmailVerified, PlatformRole: string(u.PlatformRole),
		TOTPEnabled: u.TOTPEnabled, CreatedAt: u.CreatedAt,
	}
	if u.AvatarKey != "" && s.avatars != nil {
		url, err := s.avatars.PresignGet(ctx, u.AvatarKey, "avatar", avatarDownloadTTL)
		if err != nil {
			// A missing/broken avatar object must not break the whole profile read.
			s.logger.WarnContext(ctx, "avatar presign failed", "user_id", userID, "error", err)
		} else {
			p.AvatarURL = url
		}
	}
	return p, nil
}

// UpdateName sets the display name (trimmed, 1..100 chars).
func (s *Service) UpdateName(ctx context.Context, userID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return fmt.Errorf("%w: name must be 1..100 characters", domain.ErrValidation)
	}
	return s.users.UpdateName(ctx, userID, name)
}

// RequestAvatarUpload validates the intended upload and returns the object key
// plus a presigned PUT URL. The client uploads directly to MinIO, then confirms.
func (s *Service) RequestAvatarUpload(ctx context.Context, userID, contentType string, size int64) (key, url string, err error) {
	if s.avatars == nil {
		return "", "", fmt.Errorf("%w: avatar uploads unavailable", domain.ErrForbidden)
	}
	if _, ok := allowedAvatarTypes[contentType]; !ok {
		return "", "", fmt.Errorf("%w: unsupported image type", domain.ErrValidation)
	}
	if size <= 0 || size > maxAvatarBytes {
		return "", "", fmt.Errorf("%w: avatar must be 1 byte..5 MiB", domain.ErrValidation)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("avatar key: %w", err)
	}
	key = fmt.Sprintf("avatars/%s/%s", userID, hex.EncodeToString(buf))
	url, err = s.avatars.PresignPut(ctx, key, contentType, avatarUploadTTL)
	if err != nil {
		return "", "", fmt.Errorf("avatar upload url: %w", err)
	}
	return key, url, nil
}

// ConfirmAvatar records the uploaded object key as the user's avatar. The key must
// belong to the caller's namespace (defense against confirming someone else's key).
func (s *Service) ConfirmAvatar(ctx context.Context, userID, key string) error {
	if !strings.HasPrefix(key, fmt.Sprintf("avatars/%s/", userID)) {
		return fmt.Errorf("%w: avatar key does not belong to caller", domain.ErrValidation)
	}
	return s.users.UpdateAvatarKey(ctx, userID, key)
}

// Delete removes the account after the sole-owner guard passes. It returns
// *SoleOwnerError when the caller must first reassign ownership.
func (s *Service) Delete(ctx context.Context, userID string) error {
	if s.soleOwner == nil {
		return fmt.Errorf("%w: account deletion unavailable", domain.ErrForbidden)
	}
	orgs, err := s.soleOwner.SoleOwnerOrgs(ctx, userID)
	if err != nil {
		return err
	}
	if len(orgs) > 0 {
		refs := make([]OrgRef, 0, len(orgs))
		for _, o := range orgs {
			refs = append(refs, OrgRef{ID: o.ID, Slug: o.Slug, Name: o.Name})
		}
		return &SoleOwnerError{Orgs: refs}
	}
	if err := s.sessions.RevokeAllForUser(ctx, userID, "account_deleted"); err != nil {
		return err
	}
	return s.users.Delete(ctx, userID)
}
