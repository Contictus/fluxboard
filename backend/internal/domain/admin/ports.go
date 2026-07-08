package admin

import "context"

// AdminRepository serves the cross-tenant operator views. These reads span all
// orgs, so — like the maintenance/job reads (organizations has no RLS) — the
// implementation runs on the plain/owner pool, NOT a tenant-scoped tx. This is the
// one place that deliberately bypasses the per-org RLS scope, gated by the
// platform-admin HTTP guard.
type AdminRepository interface {
	// ListTenants returns tenant summaries (with MRR + member count) per the filter.
	ListTenants(ctx context.Context, f TenantFilter) ([]TenantSummary, error)
	// GetTenant returns one org's summary; domain.ErrNotFound when absent.
	GetTenant(ctx context.Context, orgID string) (TenantSummary, error)
	// WebhookEventsForCustomer lists processed Stripe events for an org's customer,
	// newest first, bounded (FR-ADM-004). Empty customer ⇒ empty slice.
	WebhookEventsForCustomer(ctx context.Context, customerID string, limit int) ([]WebhookEvent, error)
}

// FeatureFlagRepository persists per-org feature flags ([T]; tenant tx). FR-ADM-006.
type FeatureFlagRepository interface {
	List(ctx context.Context, orgID string) ([]FeatureFlag, error)
	Set(ctx context.Context, orgID, flag string, enabled bool) error
}

// OverrideRepository persists per-org entitlement overrides ([T]; tenant tx).
// FR-ADM-002.
type OverrideRepository interface {
	List(ctx context.Context, orgID string) ([]EntitlementOverride, error)
	Set(ctx context.Context, o EntitlementOverride) error
	Delete(ctx context.Context, orgID, key string) error
}
