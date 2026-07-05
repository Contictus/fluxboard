// Package tenantuc holds the application services for organizations,
// memberships, and invitations (docs/05-TENANCY-RBAC.md). It depends only on
// domain ports; RLS/Casbin/Redis details live behind those interfaces.
package tenantuc

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

const (
	inviteTTL      = 7 * 24 * time.Hour  // pending invitation lifetime (docs/05 §5)
	slugHistoryTTL = 30 * 24 * time.Hour // 301 redirect window (FR-TEN-007)
	deleteGrace    = 14 * 24 * time.Hour // soft-delete purge grace (invariant #7)
	// RoleCacheTTL bounds authorization-revocation latency (docs/05 §6, ≤60s).
	RoleCacheTTL = 60 * time.Second
)

// slugRe: 3–40 chars, lowercase alphanumeric and hyphens, no leading/trailing
// or doubled hyphen.
var slugRe = regexp.MustCompile(`^[a-z0-9](?:-?[a-z0-9])*$`)

// Mailer sends invitation emails. Implemented in infrastructure/mailer.
type Mailer interface {
	SendInvitation(ctx context.Context, to, orgName, token string) error
}

// Deps are the collaborators the service needs.
type Deps struct {
	Orgs    tenant.OrgRepository
	Members tenant.MembershipRepository
	Invites tenant.InvitationRepository
	Users   auth.UserRepository
	Cache   tenant.MembershipCache
	Mailer  Mailer
	Logger  *slog.Logger
	Now     func() time.Time // injectable for tests; defaults to time.Now
}

// Service implements the tenancy application logic.
type Service struct {
	orgs    tenant.OrgRepository
	members tenant.MembershipRepository
	invites tenant.InvitationRepository
	users   auth.UserRepository
	cache   tenant.MembershipCache
	mailer  Mailer
	logger  *slog.Logger
	now     func() time.Time
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		orgs: d.Orgs, members: d.Members, invites: d.Invites, users: d.Users,
		cache: d.Cache, mailer: d.mailerOrNoop(), logger: logger, now: now,
	}
}

func (d Deps) mailerOrNoop() Mailer {
	if d.Mailer == nil {
		return noopMailer{}
	}
	return d.Mailer
}

type noopMailer struct{}

func (noopMailer) SendInvitation(context.Context, string, string, string) error { return nil }

func newID() string { return uuidv7.New().String() }

// ---- Organizations --------------------------------------------------------

// CreateOrg creates an org with the caller as OWNER.
func (s *Service) CreateOrg(ctx context.Context, userID, name, slug string) (*tenant.Organization, error) {
	name = strings.TrimSpace(name)
	slug = strings.ToLower(strings.TrimSpace(slug))
	if err := validateName(name); err != nil {
		return nil, err
	}
	if !slugRe.MatchString(slug) || len(slug) < 3 || len(slug) > 40 {
		return nil, fmt.Errorf("%w: slug must be 3-40 chars, lowercase letters, digits, hyphens", domain.ErrValidation)
	}
	org := &tenant.Organization{ID: newID(), Slug: slug, Name: name}
	if err := s.orgs.CreateWithOwner(ctx, org, userID); err != nil {
		return nil, err // ErrConflict on slug clash
	}
	// Warm the role cache so the creator's first tenant request is a hit.
	_ = s.cache.SetRole(ctx, org.ID, userID, tenant.RoleOwner, RoleCacheTTL)
	return s.orgs.GetByID(ctx, org.ID)
}

// ListMyOrgs returns the orgs the caller belongs to with their role.
func (s *Service) ListMyOrgs(ctx context.Context, userID string) ([]tenant.OrgMembership, error) {
	return s.orgs.ListForUser(ctx, userID)
}

// GetOrg returns an active org. Soft-deleted orgs are invisible here (see
// RestoreOrg for the grace-window path).
func (s *Service) GetOrg(ctx context.Context, orgID string) (*tenant.Organization, error) {
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if org.Deleted() {
		return nil, domain.ErrNotFound
	}
	return org, nil
}

// UpdateProfile sets the org name and (optionally) logo key (ADMIN+ gate).
func (s *Service) UpdateProfile(ctx context.Context, orgID, name string, logoKey *string) (*tenant.Organization, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := s.orgs.UpdateProfile(ctx, orgID, name, logoKey); err != nil {
		return nil, err
	}
	return s.orgs.GetByID(ctx, orgID)
}

// ChangeSlug renames the org slug and records the old slug for the 301 window
// (OWNER gate).
func (s *Service) ChangeSlug(ctx context.Context, orgID, newSlug string) (*tenant.Organization, error) {
	newSlug = strings.ToLower(strings.TrimSpace(newSlug))
	if !slugRe.MatchString(newSlug) || len(newSlug) < 3 || len(newSlug) > 40 {
		return nil, fmt.Errorf("%w: invalid slug", domain.ErrValidation)
	}
	if err := s.orgs.UpdateSlug(ctx, orgID, newSlug, s.now().Add(slugHistoryTTL)); err != nil {
		return nil, err
	}
	return s.orgs.GetByID(ctx, orgID)
}

// SoftDelete marks the org deleted with a purge deadline (OWNER gate).
func (s *Service) SoftDelete(ctx context.Context, orgID string) error {
	return s.orgs.SoftDelete(ctx, orgID, s.now().Add(deleteGrace))
}

// Restore clears a soft-delete within the grace window (OWNER gate).
func (s *Service) Restore(ctx context.Context, orgID string) error {
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		return err
	}
	if !org.Deleted() {
		return fmt.Errorf("%w: org is not deleted", domain.ErrConflict)
	}
	if org.PurgeAfter != nil && s.now().After(*org.PurgeAfter) {
		return domain.ErrNotFound // grace elapsed; treat as gone
	}
	return s.orgs.Restore(ctx, orgID)
}

// TransferOwnership moves OWNER from the caller to target (OWNER gate). Target
// must be an existing member; the caller becomes ADMIN.
func (s *Service) TransferOwnership(ctx context.Context, orgID, fromUserID, toUserID string) error {
	if fromUserID == toUserID {
		return fmt.Errorf("%w: already the owner", domain.ErrConflict)
	}
	if _, err := s.members.Get(ctx, orgID, toUserID); err != nil {
		return err // ErrNotFound → target not a member
	}
	if err := s.members.TransferOwnership(ctx, orgID, fromUserID, toUserID); err != nil {
		return err
	}
	_ = s.cache.Invalidate(ctx, orgID, fromUserID)
	_ = s.cache.Invalidate(ctx, orgID, toUserID)
	return nil
}

// ---- Members --------------------------------------------------------------

// ListMembers returns the org's members.
func (s *Service) ListMembers(ctx context.Context, orgID string) ([]tenant.Member, error) {
	return s.members.List(ctx, orgID)
}

// ChangeMemberRole sets a member's role. actingRole is the caller's role (from
// the resolved membership) for the fine-grained OWNER-protection rule
// (docs/05 §2*): ADMIN may not modify an OWNER or grant OWNER, and OWNER is
// never set via role change (use TransferOwnership).
func (s *Service) ChangeMemberRole(ctx context.Context, orgID string, actingRole tenant.OrgRole, targetUserID string, newRole tenant.OrgRole) error {
	if !newRole.Assignable() {
		return fmt.Errorf("%w: role must be ADMIN, MEMBER, or GUEST (use transfer-ownership for OWNER)", domain.ErrValidation)
	}
	target, err := s.members.Get(ctx, orgID, targetUserID)
	if err != nil {
		return err
	}
	if target.Role == tenant.RoleOwner {
		return fmt.Errorf("%w: cannot change an owner's role; transfer ownership instead", domain.ErrConflict)
	}
	if actingRole == tenant.RoleAdmin && target.Role == tenant.RoleOwner {
		return domain.ErrForbidden
	}
	if err := s.members.UpdateRole(ctx, orgID, targetUserID, newRole); err != nil {
		return err
	}
	_ = s.cache.Invalidate(ctx, orgID, targetUserID)
	return nil
}

// RemoveMember removes a member (ADMIN+ gate). Guards: ADMIN cannot remove an
// OWNER; the last OWNER cannot be removed (docs/05 §6 last_owner).
func (s *Service) RemoveMember(ctx context.Context, orgID string, actingRole tenant.OrgRole, targetUserID string) error {
	target, err := s.members.Get(ctx, orgID, targetUserID)
	if err != nil {
		return err
	}
	if target.Role == tenant.RoleOwner {
		if actingRole != tenant.RoleOwner {
			return domain.ErrForbidden
		}
		if err := s.guardLastOwner(ctx, orgID); err != nil {
			return err
		}
	}
	if err := s.members.Delete(ctx, orgID, targetUserID); err != nil {
		return err
	}
	_ = s.cache.Invalidate(ctx, orgID, targetUserID)
	return nil
}

// Leave removes the caller's own membership. The last OWNER cannot leave
// (docs/05 §6 last_owner) — they must transfer ownership or delete the org.
func (s *Service) Leave(ctx context.Context, orgID, userID string) error {
	m, err := s.members.Get(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if m.Role == tenant.RoleOwner {
		if err := s.guardLastOwner(ctx, orgID); err != nil {
			return err
		}
	}
	if err := s.members.Delete(ctx, orgID, userID); err != nil {
		return err
	}
	_ = s.cache.Invalidate(ctx, orgID, userID)
	return nil
}

// guardLastOwner returns ErrConflict (last_owner) if the org has exactly one
// OWNER.
func (s *Service) guardLastOwner(ctx context.Context, orgID string) error {
	n, err := s.members.CountByRole(ctx, orgID, tenant.RoleOwner)
	if err != nil {
		return err
	}
	if n <= 1 {
		return fmt.Errorf("%w: last_owner: the sole owner cannot leave or be removed", domain.ErrConflict)
	}
	return nil
}

func validateName(name string) error {
	if name == "" || len(name) > 100 {
		return fmt.Errorf("%w: name must be 1-100 characters", domain.ErrValidation)
	}
	return nil
}
