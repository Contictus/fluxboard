package tenant

import "time"

// OrgRole is a member's role within an organization. Ordered
// OWNER > ADMIN > MEMBER > GUEST (docs/05-TENANCY-RBAC.md §2).
type OrgRole string

const (
	RoleOwner  OrgRole = "OWNER"
	RoleAdmin  OrgRole = "ADMIN"
	RoleMember OrgRole = "MEMBER"
	RoleGuest  OrgRole = "GUEST"
)

// rank orders roles for comparisons (higher = more privileged). Unknown → -1.
func (r OrgRole) rank() int {
	switch r {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	case RoleGuest:
		return 0
	default:
		return -1
	}
}

// Valid reports whether r is a known role.
func (r OrgRole) Valid() bool { return r.rank() >= 0 }

// AtLeast reports whether r is at least as privileged as min.
func (r OrgRole) AtLeast(min OrgRole) bool { return r.rank() >= min.rank() }

// Assignable reports whether r may be granted via invitation or role change.
// OWNER is never directly assignable — ownership moves only via transfer
// (docs/05 §2, §4).
func (r OrgRole) Assignable() bool {
	return r == RoleAdmin || r == RoleMember || r == RoleGuest
}

// Organization is the tenant root. Soft-deleted orgs keep a purge_after grace
// window (invariant #7). LogoURL is transient (minted per read, never stored).
type Organization struct {
	ID               string
	Slug             string
	Name             string
	LogoKey          string
	LogoURL          string
	StripeCustomerID string
	DeletedAt        *time.Time
	PurgeAfter       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Deleted reports whether the org is soft-deleted (in its grace window).
func (o *Organization) Deleted() bool { return o.DeletedAt != nil }

// Membership binds a user to an org with a role.
type Membership struct {
	OrgID     string
	UserID    string
	Role      OrgRole
	CreatedAt time.Time
}

// OrgMembership is the "my orgs" projection: an org plus the caller's role in it.
type OrgMembership struct {
	OrgID     string
	Role      OrgRole
	Slug      string
	Name      string
	CreatedAt time.Time
}

// Member is a membership joined with the user's display fields, for the member
// list (docs/08 §4 GET /orgs/{orgId}/members).
type Member struct {
	UserID    string
	Email     string
	Name      string
	AvatarKey string
	Role      OrgRole
	CreatedAt time.Time
}

// Invitation is a pending offer to join an org. The raw token is emailed; only
// its SHA-256 is stored (docs/05 §5).
type Invitation struct {
	ID         string
	OrgID      string
	Email      string
	Role       OrgRole
	TokenHash  []byte
	InvitedBy  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Pending reports whether the invitation can still be accepted at time now.
func (i *Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && i.RevokedAt == nil && i.ExpiresAt.After(now)
}
