package adminuc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// tenantListCap bounds one admin tenant-list page.
const tenantListCap = 100

// webhookListCap bounds the per-org webhook event browser.
const webhookListCap = 100

// ImpersonationMinter issues a short-lived, read-only impersonation token carrying
// the `imp` claim (target org). Implemented by a jwtx.Signer adapter in the
// composition root; the usecase stays off the crypto package.
type ImpersonationMinter interface {
	// Mint returns a token (and its expiry) that authenticates as adminUserID with
	// an impersonation claim for targetOrg. adminSID is the admin's live session id
	// so session revocation still applies.
	Mint(adminUserID, adminSID, targetOrg string) (token string, exp time.Time, err error)
}

// WebhookRetrier re-enqueues a stored Stripe event for reprocessing (webhook:retry).
// Implemented by an Asynq adapter.
type WebhookRetrier interface {
	Enqueue(ctx context.Context, eventID string) error
}

// Deps wires the admin service.
type Deps struct {
	Tenants   admin.AdminRepository
	Flags     admin.FeatureFlagRepository
	Overrides admin.OverrideRepository
	Subs      billing.SubscriptionRepository
	Invoices  billing.InvoiceRepository
	Audit     audit.Writer
	Minter    ImpersonationMinter // nil ⇒ impersonation disabled
	Retrier   WebhookRetrier      // nil ⇒ webhook retry disabled
	Logger    *slog.Logger
	Now       func() time.Time
}

// Service implements the platform-admin application logic.
type Service struct {
	tenants   admin.AdminRepository
	flags     admin.FeatureFlagRepository
	overrides admin.OverrideRepository
	subs      billing.SubscriptionRepository
	invoices  billing.InvoiceRepository
	audit     audit.Writer
	minter    ImpersonationMinter
	retrier   WebhookRetrier
	logger    *slog.Logger
	now       func() time.Time
}

// New builds the service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{
		tenants: d.Tenants, flags: d.Flags, overrides: d.Overrides,
		subs: d.Subs, invoices: d.Invoices, audit: d.Audit,
		minter: d.Minter, retrier: d.Retrier, logger: d.Logger, now: d.Now,
	}
}

// ---- Tenants ---------------------------------------------------------------

// ListTenants returns tenant summaries per the filter (limit capped/defaulted).
func (s *Service) ListTenants(ctx context.Context, f admin.TenantFilter) ([]admin.TenantSummary, error) {
	if f.Limit <= 0 || f.Limit > tenantListCap {
		f.Limit = tenantListCap
	}
	return s.tenants.ListTenants(ctx, f)
}

// TenantDetail is the composed admin view of one org (FR-ADM-002).
type TenantDetail struct {
	Summary      admin.TenantSummary
	Subscription *billing.Subscription
	Invoices     []billing.Invoice
	Webhooks     []admin.WebhookEvent
	Overrides    []admin.EntitlementOverride
	Flags        []admin.FeatureFlag
}

// GetTenantDetail assembles the org summary + subscription timeline + webhook
// events (via the subscription's Stripe customer) + overrides + flags.
func (s *Service) GetTenantDetail(ctx context.Context, orgID string) (TenantDetail, error) {
	summary, err := s.tenants.GetTenant(ctx, orgID)
	if err != nil {
		return TenantDetail{}, err
	}
	d := TenantDetail{Summary: summary}

	if sub, err := s.subs.Get(ctx, orgID); err == nil {
		d.Subscription = sub
		if sub.StripeCustomerID != nil && *sub.StripeCustomerID != "" {
			if evs, err := s.tenants.WebhookEventsForCustomer(ctx, *sub.StripeCustomerID, webhookListCap); err != nil {
				s.logger.Warn("admin: webhook events lookup failed", "org", orgID, "err", err)
			} else {
				d.Webhooks = evs
			}
		}
	} // ErrNotFound ⇒ Free/none: no subscription, no webhooks.

	if invs, err := s.invoices.ListByOrg(ctx, orgID); err != nil {
		s.logger.Warn("admin: invoice list failed", "org", orgID, "err", err)
	} else {
		d.Invoices = invs
	}
	if ovs, err := s.overrides.List(ctx, orgID); err != nil {
		s.logger.Warn("admin: override list failed", "org", orgID, "err", err)
	} else {
		d.Overrides = ovs
	}
	if fs, err := s.flags.List(ctx, orgID); err != nil {
		s.logger.Warn("admin: flag list failed", "org", orgID, "err", err)
	} else {
		d.Flags = fs
	}
	return d, nil
}

// ---- Impersonation ---------------------------------------------------------

// StartImpersonation mints a read-only impersonation token for targetOrg and
// records a security audit row carrying BOTH identities (impersonator = admin).
func (s *Service) StartImpersonation(ctx context.Context, adminUserID, adminSID, targetOrg string) (token string, exp time.Time, err error) {
	if s.minter == nil {
		return "", time.Time{}, fmt.Errorf("%w: impersonation not configured", domain.ErrForbidden)
	}
	// Target must exist (opaque 404 otherwise).
	if _, err := s.tenants.GetTenant(ctx, targetOrg); err != nil {
		return "", time.Time{}, err
	}
	token, exp, err = s.minter.Mint(adminUserID, adminSID, targetOrg)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("mint impersonation token: %w", err)
	}
	s.append(ctx, audit.Entry{
		OrgID:              targetOrg,
		ActorUserID:        adminUserID,
		ImpersonatorUserID: adminUserID,
		Action:             audit.ActionImpersonateStart,
		TargetType:         "org",
		TargetID:           targetOrg,
		Severity:           audit.SeveritySecurity,
	})
	return token, exp, nil
}

// ---- Webhook browser + retry ----------------------------------------------

// RetryWebhook re-enqueues a stored Stripe event for reprocessing (FR-ADM-004).
func (s *Service) RetryWebhook(ctx context.Context, orgID, eventID, actor string) error {
	if s.retrier == nil {
		return fmt.Errorf("%w: webhook retry not configured", domain.ErrForbidden)
	}
	if eventID == "" {
		return fmt.Errorf("%w: event id required", domain.ErrValidation)
	}
	if err := s.retrier.Enqueue(ctx, eventID); err != nil {
		return fmt.Errorf("enqueue webhook retry: %w", err)
	}
	s.append(ctx, audit.Entry{
		OrgID: orgID, ActorUserID: actor, Action: audit.ActionWebhookRetry,
		TargetType: "stripe_event", TargetID: eventID, Severity: audit.SeverityWarning,
	})
	return nil
}

// ---- Feature flags ---------------------------------------------------------

// ListFlags returns an org's feature flags.
func (s *Service) ListFlags(ctx context.Context, orgID string) ([]admin.FeatureFlag, error) {
	return s.flags.List(ctx, orgID)
}

// SetFlag sets one org feature flag.
func (s *Service) SetFlag(ctx context.Context, orgID, flag string, enabled bool, actor string) error {
	if flag == "" {
		return fmt.Errorf("%w: flag required", domain.ErrValidation)
	}
	if err := s.flags.Set(ctx, orgID, flag, enabled); err != nil {
		return err
	}
	s.append(ctx, audit.Entry{
		OrgID: orgID, ActorUserID: actor, Action: audit.ActionFlagSet,
		TargetType: "feature_flag", TargetID: flag,
		Metadata: map[string]any{"enabled": enabled}, Severity: audit.SeverityInfo,
	})
	return nil
}

// ---- Entitlement overrides -------------------------------------------------

// ListOverrides returns an org's entitlement overrides.
func (s *Service) ListOverrides(ctx context.Context, orgID string) ([]admin.EntitlementOverride, error) {
	return s.overrides.List(ctx, orgID)
}

// SetOverride writes one entitlement override.
func (s *Service) SetOverride(ctx context.Context, o admin.EntitlementOverride) error {
	if o.Key == "" || o.Value == "" {
		return fmt.Errorf("%w: key and value required", domain.ErrValidation)
	}
	o.CreatedAt = s.now().UTC()
	if err := s.overrides.Set(ctx, o); err != nil {
		return err
	}
	s.append(ctx, audit.Entry{
		OrgID: o.OrgID, ActorUserID: o.CreatedBy, Action: audit.ActionOverrideSet,
		TargetType: "entitlement_override", TargetID: o.Key,
		Metadata: map[string]any{"value": o.Value, "note": o.Note}, Severity: audit.SeverityWarning,
	})
	return nil
}

// DeleteOverride removes one entitlement override.
func (s *Service) DeleteOverride(ctx context.Context, orgID, key, actor string) error {
	if err := s.overrides.Delete(ctx, orgID, key); err != nil {
		return err
	}
	s.append(ctx, audit.Entry{
		OrgID: orgID, ActorUserID: actor, Action: audit.ActionOverrideDelete,
		TargetType: "entitlement_override", TargetID: key, Severity: audit.SeverityWarning,
	})
	return nil
}

// append writes a best-effort audit row (never surfaced to the caller).
func (s *Service) append(ctx context.Context, e audit.Entry) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Append(ctx, e); err != nil {
		s.logger.Warn("admin audit append failed", "action", e.Action, "err", err)
	}
}
