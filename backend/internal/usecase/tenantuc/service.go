// Package tenantuc holds the application services for organizations,
// memberships, and invitations (docs/05-TENANCY-RBAC.md). It depends only on
// domain ports; RLS/Casbin/Redis details live behind those interfaces.
package tenantuc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

const (
	inviteTTL      = 7 * 24 * time.Hour  // pending invitation lifetime (docs/05 §5)
	slugHistoryTTL = 30 * 24 * time.Hour // 301 redirect window (FR-TEN-007)
	deleteGrace    = 14 * 24 * time.Hour // soft-delete purge grace (invariant #7)
	idempotencyTTL = 24 * time.Hour      // Idempotency-Key replay window (docs/08 §4)
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

// Notifier fans a notification to explicit users (invite accepted). Implemented
// by notifyuc.Service; nil disables the fan-out. Best-effort side effect.
type Notifier interface {
	NotifyTargets(ctx context.Context, orgID, actorID string, cat notify.Category, userIDs []string, title, body, entityType, entityID string)
}

// IdempotencyStore backs the Idempotency-Key replay guard on invitation creation
// (docs/08 §4). Get returns "" (no error) when the key is absent. Implemented in
// infrastructure/redis; optional — when nil the guard is a no-op.
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
}

// Deps are the collaborators the service needs.
type Deps struct {
	Orgs     tenant.OrgRepository
	Members  tenant.MembershipRepository
	Invites  tenant.InvitationRepository
	Users    auth.UserRepository
	Cache    tenant.MembershipCache
	Mailer   Mailer
	Audit    audit.Writer
	Idem     IdempotencyStore
	Events   notify.EventBus // realtime member.* events; nil ⇒ no publish
	Notifier Notifier        // invite-accepted fan-out; nil ⇒ no fan-out
	Logger   *slog.Logger
	Now      func() time.Time // injectable for tests; defaults to time.Now
}

// Service implements the tenancy application logic.
type Service struct {
	orgs     tenant.OrgRepository
	members  tenant.MembershipRepository
	invites  tenant.InvitationRepository
	users    auth.UserRepository
	cache    tenant.MembershipCache
	mailer   Mailer
	auditor  audit.Writer
	idem     IdempotencyStore
	events   notify.EventBus
	notifier Notifier
	logger   *slog.Logger
	now      func() time.Time
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
	auditor := d.Audit
	if auditor == nil {
		auditor = noopAudit{}
	}
	return &Service{
		orgs: d.Orgs, members: d.Members, invites: d.Invites, users: d.Users,
		cache: d.Cache, mailer: d.mailerOrNoop(), auditor: auditor, idem: d.Idem,
		events: d.Events, notifier: d.Notifier, logger: logger, now: now,
	}
}

// publish emits a realtime event best-effort (logged, not returned).
func (s *Service) publish(ctx context.Context, orgID, name, actorID string, data map[string]any) {
	if s.events == nil {
		return
	}
	if _, err := s.events.Publish(ctx, orgID, notify.NewEvent(name, actorID, data)); err != nil {
		s.logger.WarnContext(ctx, "event publish failed", "event", name, "err", err)
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

type noopAudit struct{}

func (noopAudit) Append(context.Context, audit.Entry) error { return nil }

func newID() string { return uuidv7.New().String() }

// writeAudit appends an audit entry best-effort, enriching actor/IP/UA from the
// request context (pkg/reqmeta). A failed write is logged, never surfaced.
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
		s.logger.WarnContext(ctx, "audit append failed", "action", e.Action, "err", err)
	}
}

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

// SlugResolution is the outcome of resolving a possibly-stale slug. Redirected
// is true when the input slug was an old one still inside the 301 window, in
// which case Org carries the canonical (current) slug.
type SlugResolution struct {
	Org        *tenant.Organization
	Redirected bool
}

// ResolveSlug maps a slug to its org, following slug_history for a stale slug
// within the 301 window (FR-TEN-007). domain.ErrNotFound when neither an active
// slug nor a live redirect matches.
func (s *Service) ResolveSlug(ctx context.Context, slug string) (SlugResolution, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return SlugResolution{}, fmt.Errorf("%w: slug required", domain.ErrValidation)
	}
	if org, err := s.orgs.GetBySlug(ctx, slug); err == nil {
		return SlugResolution{Org: org}, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return SlugResolution{}, err
	}
	orgID, err := s.orgs.SlugRedirectTarget(ctx, slug)
	if err != nil {
		return SlugResolution{}, err // ErrNotFound → 404
	}
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		return SlugResolution{}, err
	}
	if org.Deleted() {
		return SlugResolution{}, domain.ErrNotFound
	}
	return SlugResolution{Org: org, Redirected: true}, nil
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
	s.writeAudit(ctx, audit.Entry{
		OrgID:      orgID,
		Action:     audit.ActionOrgSlugChange,
		TargetType: "organization",
		TargetID:   orgID,
		Metadata:   map[string]any{"new_slug": newSlug},
		Severity:   audit.SeverityInfo,
	})
	return s.orgs.GetByID(ctx, orgID)
}

// SoftDelete marks the org deleted with a purge deadline (OWNER gate).
func (s *Service) SoftDelete(ctx context.Context, orgID string) error {
	if err := s.orgs.SoftDelete(ctx, orgID, s.now().Add(deleteGrace)); err != nil {
		return err
	}
	s.writeAudit(ctx, audit.Entry{
		OrgID:      orgID,
		Action:     audit.ActionOrgSoftDelete,
		TargetType: "organization",
		TargetID:   orgID,
		Severity:   audit.SeverityWarning,
	})
	return nil
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
	if err := s.orgs.Restore(ctx, orgID); err != nil {
		return err
	}
	s.writeAudit(ctx, audit.Entry{
		OrgID:      orgID,
		Action:     audit.ActionOrgRestore,
		TargetType: "organization",
		TargetID:   orgID,
		Severity:   audit.SeverityInfo,
	})
	return nil
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
	s.writeAudit(ctx, audit.Entry{
		OrgID:       orgID,
		ActorUserID: fromUserID,
		Action:      audit.ActionOrgTransferOwnership,
		TargetType:  "user",
		TargetID:    toUserID,
		Severity:    audit.SeveritySecurity,
	})
	return nil
}

// ---- Members --------------------------------------------------------------

// MemberPage is a filtered, keyset-paginated slice of members.
type MemberPage struct {
	Items      []tenant.Member
	NextCursor string
}

// ListMembers returns the org's members filtered by optional role/text and
// keyset-paginated (docs/08 §4 ?role=&q=). limit is clamped to [1,100].
func (s *Service) ListMembers(ctx context.Context, orgID string, role *tenant.OrgRole, q, cursor string, limit int) (MemberPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	mq := tenant.MemberQuery{Role: role, Q: strings.TrimSpace(q), Limit: limit + 1} // +1 to detect a next page
	if cursor != "" {
		ac, au, err := decodeMemberCursor(cursor)
		if err != nil {
			return MemberPage{}, fmt.Errorf("%w: invalid cursor", domain.ErrValidation)
		}
		mq.AfterCreated, mq.AfterUser = ac, au
	}
	items, err := s.members.List(ctx, orgID, mq)
	if err != nil {
		return MemberPage{}, err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = encodeMemberCursor(last.CreatedAt, last.UserID)
	}
	return MemberPage{Items: items, NextCursor: next}, nil
}

// encodeMemberCursor / decodeMemberCursor serialize the (created_at, user_id)
// keyset position as an opaque base64 token.
func encodeMemberCursor(createdAt time.Time, userID string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + userID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMemberCursor(cursor string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
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
	s.writeAudit(ctx, audit.Entry{
		OrgID:      orgID,
		Action:     audit.ActionMemberRoleChange,
		TargetType: "user",
		TargetID:   targetUserID,
		Metadata:   map[string]any{"old_role": string(target.Role), "new_role": string(newRole)},
		Severity:   audit.SeveritySecurity,
	})
	s.publish(ctx, orgID, notify.EventMemberRoleChanged, "", map[string]any{
		"user_id": targetUserID, "role": string(newRole),
	})
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
	s.writeAudit(ctx, audit.Entry{
		OrgID:      orgID,
		Action:     audit.ActionMemberRemove,
		TargetType: "user",
		TargetID:   targetUserID,
		Severity:   audit.SeverityWarning,
	})
	// membership.revoked is targeted: the removed user's client hard-redirects
	// out of the org (09 §1). Also broadcast member.left for the roster.
	s.publish(ctx, orgID, notify.EventMembershipRevoked, "", map[string]any{"user_id": targetUserID})
	s.publish(ctx, orgID, notify.EventMemberLeft, "", map[string]any{"user_id": targetUserID})
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
	s.writeAudit(ctx, audit.Entry{
		OrgID:       orgID,
		ActorUserID: userID,
		Action:      audit.ActionMemberLeave,
		TargetType:  "user",
		TargetID:    userID,
		Severity:    audit.SeverityInfo,
	})
	s.publish(ctx, orgID, notify.EventMemberLeft, userID, map[string]any{"user_id": userID})
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
