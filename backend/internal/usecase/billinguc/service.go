// Package billinguc holds the application services for subscriptions, checkout,
// plan changes, entitlements and the Stripe webhook consumer (docs/06-BILLING.md).
// It depends only on the domain ports in internal/domain/billing; Stripe is the
// source of truth for subscription state, so most lifecycle methods just trigger
// a gateway call and bust the entitlement cache — the local mirror is updated by
// the webhook (see webhook.go). Money is integer minor units; times are UTC.
package billinguc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// entTTL is the entitlement cache lifetime (06 §7). The write-through Bust on
// webhook + plan change makes this only a safety net.
const entTTL = 60 * time.Second

// Deps are the collaborators the service needs — all domain ports.
type Deps struct {
	Plans    billing.PlanRepository
	Subs     billing.SubscriptionRepository
	Invoices billing.InvoiceRepository
	Events   billing.ProcessedEventRepository
	Webhooks billing.WebhookRepository
	Usage    billing.UsageRepository
	Gateway  billing.StripeGateway
	Cache    billing.EntitlementCache
	Bus      notify.EventBus // realtime billing.status_changed; nil ⇒ no publish
	Logger   *slog.Logger
	Now      func() time.Time // injectable for tests; defaults to time.Now
	BaseURL  string           // public app base, for Checkout/Portal return URLs
}

// Service implements the billing application logic.
type Service struct {
	plans    billing.PlanRepository
	subs     billing.SubscriptionRepository
	invoices billing.InvoiceRepository
	events   billing.ProcessedEventRepository
	webhooks billing.WebhookRepository
	usage    billing.UsageRepository
	gw       billing.StripeGateway
	cache    billing.EntitlementCache
	bus      notify.EventBus
	logger   *slog.Logger
	now      func() time.Time
	baseURL  string
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
		plans: d.Plans, subs: d.Subs, invoices: d.Invoices, events: d.Events,
		webhooks: d.Webhooks, usage: d.Usage, gw: d.Gateway, cache: d.Cache,
		bus: d.Bus, logger: logger, now: now, baseURL: d.BaseURL,
	}
}

func newID() string { return uuidv7.New().String() }

func strPtr(s string) *string { return &s }

// ---- Views ----------------------------------------------------------------

// SummaryView is the billing page snapshot (FR-BILL-001). Usage-vs-limits for
// members/projects/storage is composed at the HTTP layer from the owning
// services (those counts live in the project/tenant domains, not billing); this
// view carries the resolved entitlement limits and subscription meta.
type SummaryView struct {
	Plan              billing.PlanCode
	Status            billing.SubStatus
	Entitlements      billing.Entitlements
	CurrentPeriodEnd  *time.Time
	CancelAtPeriodEnd bool
	PastDueWarning    bool
	HasSubscription   bool
}

// ---- Entitlements ---------------------------------------------------------

// Resolve returns an org's entitlement envelope, cache-first. On a miss it
// derives from the subscription mirror (or Free when the org has no row) and
// caches the result for entTTL. Shared by Summary and the entitlement
// middleware (§5). FR-BILL-009, 06 §7.
func (s *Service) Resolve(ctx context.Context, orgID string) (billing.Entitlements, error) {
	if e, ok, err := s.cache.Get(ctx, orgID); err != nil {
		s.logger.WarnContext(ctx, "entitlement cache get failed", "org", orgID, "err", err)
	} else if ok {
		return e, nil
	}

	free, err := s.plans.Get(ctx, billing.PlanFree)
	if err != nil {
		return billing.Entitlements{}, fmt.Errorf("load free plan: %w", err)
	}

	var ent billing.Entitlements
	sub, err := s.subs.Get(ctx, orgID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		ent = billing.DeriveEntitlements(*free, *free, billing.StatusNone)
	case err != nil:
		return billing.Entitlements{}, err
	default:
		subscribed, perr := s.plans.Get(ctx, sub.PlanCode)
		if perr != nil {
			return billing.Entitlements{}, fmt.Errorf("load subscribed plan %s: %w", sub.PlanCode, perr)
		}
		ent = billing.DeriveEntitlements(*subscribed, *free, sub.Status)
	}

	if err := s.cache.Set(ctx, orgID, ent, entTTL); err != nil {
		s.logger.WarnContext(ctx, "entitlement cache set failed", "org", orgID, "err", err)
	}
	return ent, nil
}

// Summary returns the billing page snapshot for an org. FR-BILL-001.
func (s *Service) Summary(ctx context.Context, orgID string) (SummaryView, error) {
	ent, err := s.Resolve(ctx, orgID)
	if err != nil {
		return SummaryView{}, err
	}
	v := SummaryView{
		Plan: ent.Plan, Status: ent.Status, Entitlements: ent,
		PastDueWarning: ent.PastDueWarning,
	}
	sub, err := s.subs.Get(ctx, orgID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// no subscription → Free/none; view already reflects that.
	case err != nil:
		return SummaryView{}, err
	default:
		v.HasSubscription = true
		v.CurrentPeriodEnd = sub.CurrentPeriodEnd
		v.CancelAtPeriodEnd = sub.CancelAtPeriodEnd
	}
	return v, nil
}

// ListInvoices returns an org's invoice mirror, newest-first. FR-BILL-008.
func (s *Service) ListInvoices(ctx context.Context, orgID string) ([]billing.Invoice, error) {
	return s.invoices.ListByOrg(ctx, orgID)
}

// ---- Subscription lifecycle -----------------------------------------------

// Checkout starts a hosted Stripe Checkout for a paid plan (FR-BILL-002, 06 §3).
// It lazily creates+persists the org's Stripe customer, then returns a Checkout
// URL. The subscription mirror is not written here — it flips to active when the
// checkout.session.completed webhook arrives.
func (s *Service) Checkout(ctx context.Context, orgID string, planCode billing.PlanCode, seats int64) (billing.CheckoutSession, error) {
	if !planCode.Valid() || planCode == billing.PlanFree {
		return billing.CheckoutSession{}, domain.ErrValidation
	}
	if seats < 1 {
		seats = 1
	}
	plan, err := s.plans.Get(ctx, planCode)
	if err != nil {
		return billing.CheckoutSession{}, err
	}

	existing, sub, err := s.currentCustomer(ctx, orgID)
	if err != nil {
		return billing.CheckoutSession{}, err
	}
	customerID, err := s.gw.EnsureCustomer(ctx, orgID, existing)
	if err != nil {
		return billing.CheckoutSession{}, fmt.Errorf("ensure customer: %w", err)
	}
	if existing == "" {
		if err := s.persistCustomer(ctx, orgID, sub, customerID); err != nil {
			return billing.CheckoutSession{}, err
		}
	}

	idemKey := fmt.Sprintf("co:%s:%s:%d", orgID, planCode, s.now().Unix()/300)
	return s.gw.CreateCheckout(ctx, billing.CheckoutParams{
		OrgID:          orgID,
		Plan:           *plan,
		Seats:          seats,
		CustomerID:     customerID,
		IdempotencyKey: idemKey,
		SuccessURL:     s.baseURL + "/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:      s.baseURL + "/billing/checkout/cancelled",
	})
}

// PreviewChange returns the prorated amount (minor units) + currency for moving
// to planCode, without applying it. FR-BILL-003.
func (s *Service) PreviewChange(ctx context.Context, orgID string, planCode billing.PlanCode) (int64, string, error) {
	plan, sub, err := s.paidChangeTarget(ctx, orgID, planCode)
	if err != nil {
		return 0, "", err
	}
	// TODO(seats): seat quantity isn't mirrored locally; preview current seats.
	return s.gw.UpcomingInvoice(ctx, *sub.StripeSubscriptionID, *plan, 1)
}

// ApplyChange switches the org to planCode with create_prorations under an
// idempotency key, then busts the entitlement cache. The mirror updates via the
// customer.subscription.updated webhook. FR-BILL-003.
func (s *Service) ApplyChange(ctx context.Context, orgID string, planCode billing.PlanCode) error {
	plan, sub, err := s.paidChangeTarget(ctx, orgID, planCode)
	if err != nil {
		return err
	}
	idemKey := fmt.Sprintf("chg:%s:%s:%d", orgID, planCode, s.now().Unix()/300)
	// TODO(seats): preserve current seat quantity once mirrored.
	if err := s.gw.UpdateSubscription(ctx, *sub.StripeSubscriptionID, *plan, 1, idemKey); err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	s.bust(ctx, orgID)
	return nil
}

// Cancel cancels the subscription immediately or at period end, then busts the
// cache. Immediate cancel is admin-gated at the HTTP layer (Phase 6). FR-BILL-004.
func (s *Service) Cancel(ctx context.Context, orgID string, atPeriodEnd bool) error {
	sub, err := s.subs.Get(ctx, orgID)
	if err != nil {
		return err
	}
	if sub.StripeSubscriptionID == nil {
		return domain.ErrConflict
	}
	if err := s.gw.CancelSubscription(ctx, *sub.StripeSubscriptionID, atPeriodEnd); err != nil {
		return fmt.Errorf("cancel subscription: %w", err)
	}
	s.bust(ctx, orgID)
	return nil
}

// Resume clears a scheduled at-period-end cancellation, then busts the cache.
// FR-BILL-004.
func (s *Service) Resume(ctx context.Context, orgID string) error {
	sub, err := s.subs.Get(ctx, orgID)
	if err != nil {
		return err
	}
	if sub.StripeSubscriptionID == nil {
		return domain.ErrConflict
	}
	if err := s.gw.Resume(ctx, *sub.StripeSubscriptionID); err != nil {
		return fmt.Errorf("resume subscription: %w", err)
	}
	s.bust(ctx, orgID)
	return nil
}

// Portal returns a Stripe Billing Portal URL for payment-method management
// (FR-BILL-006). Requires an existing Stripe customer.
func (s *Service) Portal(ctx context.Context, orgID string) (string, error) {
	sub, err := s.subs.Get(ctx, orgID)
	if err != nil {
		return "", err
	}
	if sub.StripeCustomerID == nil {
		return "", domain.ErrConflict
	}
	return s.gw.PortalSession(ctx, *sub.StripeCustomerID, s.baseURL+"/billing")
}

// ---- helpers --------------------------------------------------------------

// currentCustomer returns the org's existing Stripe customer id ("" if none)
// and its subscription row (nil if none), tolerating ErrNotFound.
func (s *Service) currentCustomer(ctx context.Context, orgID string) (string, *billing.Subscription, error) {
	sub, err := s.subs.Get(ctx, orgID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "", nil, nil
	case err != nil:
		return "", nil, err
	}
	if sub.StripeCustomerID != nil {
		return *sub.StripeCustomerID, sub, nil
	}
	return "", sub, nil
}

// persistCustomer stores a lazily created Stripe customer id on the org's
// subscription row, creating a minimal Free/none row if none exists yet.
func (s *Service) persistCustomer(ctx context.Context, orgID string, sub *billing.Subscription, customerID string) error {
	now := s.now().UTC()
	if sub == nil {
		sub = &billing.Subscription{
			ID: newID(), OrgID: orgID, PlanCode: billing.PlanFree,
			Status: billing.StatusNone, CreatedAt: now,
		}
	}
	sub.StripeCustomerID = &customerID
	sub.UpdatedAt = now
	return s.subs.Upsert(ctx, orgID, sub)
}

// paidChangeTarget validates a paid plan-change request and returns the target
// plan + the org's subscription (which must have a live Stripe subscription id).
func (s *Service) paidChangeTarget(ctx context.Context, orgID string, planCode billing.PlanCode) (*billing.Plan, *billing.Subscription, error) {
	if !planCode.Valid() || planCode == billing.PlanFree {
		return nil, nil, domain.ErrValidation
	}
	plan, err := s.plans.Get(ctx, planCode)
	if err != nil {
		return nil, nil, err
	}
	sub, err := s.subs.Get(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	if sub.StripeSubscriptionID == nil {
		return nil, nil, domain.ErrConflict // no active subscription to change
	}
	return plan, sub, nil
}

// bust deletes the entitlement cache entry (best-effort; a miss just re-derives).
func (s *Service) bust(ctx context.Context, orgID string) {
	if err := s.cache.Bust(ctx, orgID); err != nil {
		s.logger.WarnContext(ctx, "entitlement cache bust failed", "org", orgID, "err", err)
	}
}
