package tenant

import (
	"context"
	"time"
)

// OrgRepository persists organizations. organizations has NO RLS (it must be
// readable to resolve tenant context), so this uses the global pool. The "my
// orgs" and invitation-by-token reads go through sanctioned SECURITY DEFINER
// functions (docs/05 §4, ADR-014).
type OrgRepository interface {
	// CreateWithOwner inserts the org and its creator's OWNER membership in one
	// transaction (organizations has no RLS; the membership insert sets the
	// tenant GUC within the same tx). Returns domain.ErrConflict on slug clash.
	CreateWithOwner(ctx context.Context, o *Organization, ownerUserID string) error
	// GetByID returns domain.ErrNotFound when absent. Soft-deleted orgs are
	// returned (callers decide) so restore/az can see them.
	GetByID(ctx context.Context, id string) (*Organization, error)
	// GetBySlug returns domain.ErrNotFound when absent (excludes soft-deleted).
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	// SlugRedirectTarget resolves a stale slug to its org id via slug_history
	// within the 30-day 301 window (FR-TEN-007). domain.ErrNotFound when absent
	// or expired.
	SlugRedirectTarget(ctx context.Context, oldSlug string) (string, error)
	// ListForUser returns every org the user belongs to with their role.
	ListForUser(ctx context.Context, userID string) ([]OrgMembership, error)
	// UpdateProfile sets name and (optionally) logo. logoKey nil leaves it.
	UpdateProfile(ctx context.Context, id, name string, logoKey *string) error
	// UpdateSlug changes the slug and records the old one in slug_history for
	// the 301 window, in one transaction. Returns domain.ErrConflict if taken.
	UpdateSlug(ctx context.Context, id, newSlug string, historyExpires time.Time) error
	// SoftDelete marks the org deleted with a purge deadline.
	SoftDelete(ctx context.Context, id string, purgeAfter time.Time) error
	// Restore clears the soft-delete within the grace window.
	Restore(ctx context.Context, id string) error
}

// MembershipRepository persists memberships ([T], RLS-isolated). Every method
// takes orgID; the implementation opens a tenant-scoped transaction (SET LOCAL
// app.current_tenant) so RLS is the backstop under the application filter.
type MembershipRepository interface {
	// Get returns the user's membership, or domain.ErrNotFound.
	Get(ctx context.Context, orgID, userID string) (*Membership, error)
	// List returns members joined with user display fields, filtered and keyset-
	// paginated per q (docs/08 §4 ?role=&q=). Ordered by (created_at, user_id).
	List(ctx context.Context, orgID string, q MemberQuery) ([]Member, error)
	// UpdateRole changes a member's role. Returns domain.ErrNotFound if absent.
	UpdateRole(ctx context.Context, orgID, userID string, role OrgRole) error
	// Delete removes a membership. Returns domain.ErrNotFound if absent.
	Delete(ctx context.Context, orgID, userID string) error
	// CountByRole counts members holding role (last-owner guard, docs/05 §6).
	CountByRole(ctx context.Context, orgID string, role OrgRole) (int, error)
	// TransferOwnership demotes the current owner to ADMIN and promotes target
	// to OWNER in one transaction. Returns domain.ErrNotFound if target absent.
	TransferOwnership(ctx context.Context, orgID, fromUserID, toUserID string) error
	// AcceptInvitation inserts the membership and marks the invitation accepted
	// in ONE tenant-scoped transaction (docs/05 §5). Returns domain.ErrConflict
	// if already a member, domain.ErrNotFound/ErrConflict if the invite is gone.
	AcceptInvitation(ctx context.Context, orgID, invitationID, userID string, role OrgRole) error
}

// MemberQuery filters and paginates a member list (docs/08 §4). Role/Q are
// optional filters; AfterCreated+AfterUser are the keyset cursor (exclusive);
// Limit caps the page. Zero AfterCreated means "from the start".
type MemberQuery struct {
	Role         *OrgRole
	Q            string
	AfterCreated time.Time
	AfterUser    string
	Limit        int
}

// InvitationRepository persists invitations ([T], RLS-isolated). Per-org reads
// take orgID and run tenant-scoped; the accept flow resolves the org from the
// token via a SECURITY DEFINER function before opening tenant context.
type InvitationRepository interface {
	Create(ctx context.Context, inv *Invitation) error
	// Get returns a single invitation by id, or domain.ErrNotFound.
	Get(ctx context.Context, orgID, id string) (*Invitation, error)
	// GetByEmail returns the pending-or-any invitation for an address in the
	// org, or domain.ErrNotFound (duplicate-invite guard).
	GetByEmail(ctx context.Context, orgID, email string) (*Invitation, error)
	// ListPending returns invitations not yet accepted/revoked/expired.
	ListPending(ctx context.Context, orgID string) ([]Invitation, error)
	// ResolveByToken reads an invitation across tenants by token hash (accept
	// flow). Returns domain.ErrNotFound if unknown.
	ResolveByToken(ctx context.Context, tokenHash []byte) (*Invitation, error)
	// Revoke marks the invitation revoked. Returns domain.ErrNotFound if absent.
	Revoke(ctx context.Context, orgID, id string) error
	// UpdateToken rotates the token hash and expiry (resend).
	UpdateToken(ctx context.Context, orgID, id string, tokenHash []byte, expiresAt time.Time) error
}

// MembershipCache bounds authorization-revocation latency: a removed member's
// live requests must 403 within the TTL (docs/05 §6, ≤60s). The middleware
// reads the caller's role from here and backfills from Postgres on a miss.
type MembershipCache interface {
	// GetRole returns (role, known). A miss is ("", false); the caller
	// backfills and calls SetRole.
	GetRole(ctx context.Context, orgID, userID string) (role OrgRole, known bool, err error)
	// SetRole caches the resolved role for ttl.
	SetRole(ctx context.Context, orgID, userID string, role OrgRole, ttl time.Duration) error
	// Invalidate drops the cached role so the next request re-resolves (called
	// on role change / removal).
	Invalidate(ctx context.Context, orgID, userID string) error
}

// Authorizer answers the coarse org-role → (object, action) gate (Casbin,
// docs/05 §3). Fine-grained resource checks stay in the usecase layer.
type Authorizer interface {
	// Allowed reports whether an org role may perform action on object.
	Allowed(role OrgRole, object, action string) (bool, error)
}
