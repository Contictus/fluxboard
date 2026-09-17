// Package admin is the platform-operator domain (docs/build/PHASE-6-ADMIN-OBS.md
// §2, FR-ADM-001..006). It models the cross-tenant views a platform admin sees —
// tenant summaries with MRR, per-org webhook events, feature flags and entitlement
// overrides — plus the read-only impersonation claim. Platform role lives in the
// auth domain (auth.PlatformRole); this package does not redefine it. Stdlib-only.
package admin

import "time"

// TenantFilter parameters the admin tenant list (FR-ADM-002). Empty fields are
// ignored. Search matches org name or slug. Plan/Status filter on the org's
// subscription. Limit is capped by the usecase.
type TenantFilter struct {
	Search string
	Plan   string
	Status string
	Limit  int
	Offset int
}

// TenantSummary is one row of the admin tenant list: identity, plan/status,
// member count, and monthly recurring revenue in minor units (SUM of the plan's
// monthly_price over the org's active/trialing subscription; 0 for Free/none).
type TenantSummary struct {
	OrgID       string
	Name        string
	Slug        string
	PlanCode    string
	Status      string
	MemberCount int
	MRR         int64 // minor units (invariant #5)
	CreatedAt   time.Time
}

// WebhookEvent mirrors a processed_stripe_events row for the admin webhook browser
// (FR-ADM-004). Handled=false marks an event recognized but with no handler (a
// retry candidate). Error carries the last processing error, if any.
type WebhookEvent struct {
	EventID     string    `json:"event_id"`
	Type        string    `json:"type"`
	Handled     bool      `json:"handled"`
	Error       string    `json:"error"`
	ProcessedAt time.Time `json:"processed_at"`
}

// EntitlementOverride is a platform-admin override of one entitlement field for a
// single org (FR-ADM-002). Value is stored as text and parsed by the resolver.
type EntitlementOverride struct {
	OrgID     string    `json:"org_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// FeatureFlag is a per-org toggle (FR-ADM-006). An absent row ⇒ disabled.
type FeatureFlag struct {
	OrgID   string `json:"org_id"`
	Flag    string `json:"flag"`
	Enabled bool   `json:"enabled"`
}

// ImpersonationClaim is the read-only dual-identity marker an admin mints to view
// a tenant as one of its members (FR-ADM-003). It grants READ access to the target
// org only; the write-guard rejects any mutating request carrying it, and every
// impersonated request is audit-logged with both identities.
type ImpersonationClaim struct {
	AdminUserID string
	TargetOrgID string
}
