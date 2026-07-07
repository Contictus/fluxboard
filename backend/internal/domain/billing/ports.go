package billing

import (
	"context"
	"time"
)

// Repositories persist billing data. Tenant-owned ([T]) tables — subscriptions,
// invoices, usage_records, outbox — take orgID first and open a tenant-scoped
// transaction (RLS backstop). The global tables — plans, processed_stripe_events
// — are read/written on the plain pool (no tenant context; the webhook endpoint
// has none). Reads return domain.ErrNotFound when absent.

// PlanRepository reads the global plans reference table.
type PlanRepository interface {
	// Get returns a plan by code, or domain.ErrNotFound.
	Get(ctx context.Context, code PlanCode) (*Plan, error)
	// List returns all plans (for admin + client plan tables).
	List(ctx context.Context) ([]Plan, error)
}

// SubscriptionRepository persists the per-org subscription mirror.
type SubscriptionRepository interface {
	// Get returns an org's subscription, or domain.ErrNotFound if it has none
	// (which the caller treats as Free/none).
	Get(ctx context.Context, orgID string) (*Subscription, error)
	// Upsert inserts or updates the org's subscription row (keyed on org_id).
	// Used for lazy customer creation and reconciliation; webhook-driven state
	// changes go through WebhookRepository.Apply to stay in one tx with dedup.
	Upsert(ctx context.Context, orgID string, s *Subscription) error
}

// InvoiceRepository persists the invoice mirror (FR-BILL-008).
type InvoiceRepository interface {
	// Upsert inserts or updates an invoice by stripe_invoice_id.
	Upsert(ctx context.Context, orgID string, inv *Invoice) error
	// ListByOrg returns an org's invoices newest-first.
	ListByOrg(ctx context.Context, orgID string) ([]Invoice, error)
}

// ProcessedEventRepository writes the GLOBAL webhook idempotency ledger for
// events that cannot be routed to an org (recorded handled=false). Routable
// events are deduped inside WebhookRepository.Apply's transaction instead.
type ProcessedEventRepository interface {
	// Record inserts a ledger row; inserted=false means the event_id was already
	// present (duplicate). Runs on the plain pool (global table, no RLS).
	Record(ctx context.Context, e ProcessedEvent) (inserted bool, err error)
}

// UsageRepository persists per-(org, metric, day) usage aggregates (FR-BILL-007).
type UsageRepository interface {
	// Upsert idempotently sets the aggregate for (org, metric, period_date).
	Upsert(ctx context.Context, orgID string, r UsageRecord) error
	// ListForPush returns metered records for the given day not yet pushed to
	// Stripe (daily push job).
	ListForPush(ctx context.Context, orgID string, day time.Time) ([]UsageRecord, error)
	// MarkPushed stamps pushed_at on a record after a successful Stripe push.
	MarkPushed(ctx context.Context, orgID string, metric UsageMetric, day time.Time, at time.Time) error
}

// OutboxRepository drains the transactional outbox (09 §2). Producers insert
// rows inside their own write transaction (the webhook does so via
// WebhookRepository.Apply); the worker claims + drains them per-org.
type OutboxRepository interface {
	// ClaimBatch atomically marks up to limit undrained rows drained and returns
	// them (UPDATE … WHERE drained_at IS NULL RETURNING). The caller enqueues each
	// to Asynq with TaskID = row id (dedup).
	ClaimBatch(ctx context.Context, orgID string, limit int) ([]OutboxItem, error)
}

// WebhookMutation is the complete, atomic set of writes a single Stripe event
// produces (docs/06-BILLING.md §4). WebhookRepository.Apply performs the dedup
// insert, the staleness check, and every side effect in ONE transaction so the
// invariant "processed at most once, in the same tx as its side effects" holds.
type WebhookMutation struct {
	Event ProcessedEvent // ledger row; dedup on Event.EventID
	// EventCreated is the Stripe event time used for the out-of-order staleness
	// guard against subscriptions.last_stripe_event_at.
	EventCreated time.Time
	// Subscription, when non-nil, is the desired full subscription state to
	// upsert (skipped if the staleness guard trips).
	Subscription *Subscription
	// Invoice, when non-nil, is upserted (invoice events are not staleness-gated).
	Invoice *Invoice
	// Outbox rows are enqueued in the same tx (e.g. dunning email).
	Outbox []OutboxItem
}

// WebhookResult reports what Apply did, so the usecase can decide follow-ups
// (e.g. bust the entitlement cache only when subscription state actually moved).
type WebhookResult struct {
	Duplicate      bool // event_id already processed → no side effects
	Stale          bool // event older than last_stripe_event_at → mutation skipped
	SubscriptionCh bool // subscription row was inserted/updated
}

// WebhookRepository applies a WebhookMutation atomically for one org.
type WebhookRepository interface {
	Apply(ctx context.Context, orgID string, m WebhookMutation) (WebhookResult, error)
}

// ---- Stripe gateway (adapter port) ----------------------------------------

// The gateway abstracts Stripe so the usecase never touches stripe-go. The stub
// implementation (MODE=stub) fakes it with no external calls; the live
// implementation wraps stripe-go/v78 behind the same interface.

// CheckoutParams describes a hosted-Checkout session request (06 §3).
type CheckoutParams struct {
	OrgID          string
	Plan           Plan
	Seats          int64
	CustomerID     string // existing Stripe customer, or "" to create lazily
	IdempotencyKey string
	SuccessURL     string
	CancelURL      string
}

// CheckoutSession is the result of CreateCheckout.
type CheckoutSession struct {
	URL        string
	CustomerID string // the resolved/created customer id (persist it on the org)
}

// StripeSubscription is the gateway-normalized subscription view carried on a
// checkout/subscription event so the usecase works in domain types, not raw JSON.
type StripeSubscription struct {
	SubscriptionID    string
	CustomerID        string
	Status            SubStatus
	PlanCode          PlanCode
	CurrentPeriodEnd  *time.Time
	CancelAtPeriodEnd bool
}

// StripeInvoice is the gateway-normalized invoice view on an invoice.* event.
type StripeInvoice struct {
	InvoiceID    string
	Number       string
	Status       string
	AmountDue    int64
	AmountPaid   int64
	Currency     string
	HostedPDFURL string
	PeriodStart  *time.Time
	PeriodEnd    *time.Time
}

// StripeEvent is a signature-verified, gateway-normalized webhook event. The
// adapter resolves OrgID from the object metadata (or customer lookup); an empty
// OrgID means the event is unroutable (recorded handled=false, no side effects).
type StripeEvent struct {
	ID      string
	Type    string
	Created time.Time
	OrgID   string
	Sub     *StripeSubscription // set for checkout.session.completed + subscription.*
	Invoice *StripeInvoice      // set for invoice.*
}

// StripeGateway is the outbound Stripe port (06 §3–§5).
type StripeGateway interface {
	// EnsureCustomer returns the org's Stripe customer id, creating one (with
	// metadata org_id) if existing is empty.
	EnsureCustomer(ctx context.Context, orgID, existing string) (string, error)
	// CreateCheckout returns a hosted Checkout URL (+ resolved customer id).
	CreateCheckout(ctx context.Context, p CheckoutParams) (CheckoutSession, error)
	// UpcomingInvoice returns the prorated amount (minor units) + currency for a
	// plan change preview (FR-BILL-003).
	UpcomingInvoice(ctx context.Context, subscriptionID string, newPlan Plan, seats int64) (amount int64, currency string, err error)
	// UpdateSubscription changes the plan with create_prorations under idemKey.
	UpdateSubscription(ctx context.Context, subscriptionID string, newPlan Plan, seats int64, idemKey string) error
	// CancelSubscription cancels immediately or at period end.
	CancelSubscription(ctx context.Context, subscriptionID string, atPeriodEnd bool) error
	// Resume clears a scheduled at-period-end cancellation.
	Resume(ctx context.Context, subscriptionID string) error
	// PortalSession returns a Billing Portal URL for payment-method management.
	PortalSession(ctx context.Context, customerID, returnURL string) (string, error)
	// PushUsage reports a metered aggregate to Stripe with action=set (06 §5).
	PushUsage(ctx context.Context, subscriptionID string, metric UsageMetric, quantity int64, ts time.Time) error
	// FetchSubscription reads the authoritative remote subscription state for
	// the nightly reconcile (06 §8, ADR-010). Returns ErrReconcileUnsupported
	// when the gateway has no remote to read (MODE=stub).
	FetchSubscription(ctx context.Context, subscriptionID string) (*StripeSubscription, error)
	// ConstructEvent verifies the signature and normalizes the payload.
	// Returns domain.ErrValidation on a bad/expired signature.
	ConstructEvent(payload []byte, sigHeader string) (StripeEvent, error)
}

// EntitlementCache is the Redis-backed entitlement cache (06 §7, TTL 60s,
// write-through Bust on webhook + plan change).
type EntitlementCache interface {
	// Get returns cached entitlements; ok=false on a miss.
	Get(ctx context.Context, orgID string) (e Entitlements, ok bool, err error)
	// Set caches entitlements with ttl.
	Set(ctx context.Context, orgID string, e Entitlements, ttl time.Duration) error
	// Bust deletes the cache entry.
	Bust(ctx context.Context, orgID string) error
}
