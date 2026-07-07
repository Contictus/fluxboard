// Package billing models the Phase 4 billing domain: Plan, Subscription,
// Invoice, UsageRecord and the entitlement derivation state machine, plus their
// ports. Stdlib-only (docs/03-ARCHITECTURE.md ADR-001); Stripe is the source of
// truth for subscription state (docs/06-BILLING.md §2). Money is always integer
// minor units (invariant #5); times are UTC.
package billing

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrReconcileUnsupported is returned by StripeGateway.FetchSubscription when
// the gateway cannot read remote subscription state (MODE=stub has no remote —
// there is nothing to drift from). The reconcile job skips the org on it.
var ErrReconcileUnsupported = errors.New("billing: gateway does not support subscription fetch")

// ---- Plans ----------------------------------------------------------------

// PlanCode identifies a subscription tier (docs/06-BILLING.md §1).
type PlanCode string

const (
	PlanFree     PlanCode = "free"
	PlanPro      PlanCode = "pro"
	PlanBusiness PlanCode = "business"
)

// Valid reports whether c is a known plan code.
func (c PlanCode) Valid() bool {
	switch c {
	case PlanFree, PlanPro, PlanBusiness:
		return true
	default:
		return false
	}
}

// Plan is a tier's reference data: its Stripe price wiring plus the entitlement
// limits it grants (docs/06-BILLING.md §7). Limits use -1 for "unlimited".
type Plan struct {
	Code                  PlanCode
	Name                  string
	StripeProductID       string
	SeatPriceID           string
	MeteredStoragePriceID string
	MeteredAPIPriceID     string
	MaxMembers            int
	MaxProjects           int
	MaxStorageBytes       int64
	APIRatePerMin         int
	AuditRetentionDays    int
	Metered               bool
}

// ---- Subscription state machine -------------------------------------------

// SubStatus is the local mirror of the Stripe subscription status
// (docs/06-BILLING.md §2). 'none' means the org has no paid subscription.
type SubStatus string

const (
	StatusNone     SubStatus = "none"
	StatusTrialing SubStatus = "trialing"
	StatusActive   SubStatus = "active"
	StatusPastDue  SubStatus = "past_due"
	StatusUnpaid   SubStatus = "unpaid"
	StatusCanceled SubStatus = "canceled"
)

// Valid reports whether s is a known status.
func (s SubStatus) Valid() bool {
	switch s {
	case StatusNone, StatusTrialing, StatusActive, StatusPastDue, StatusUnpaid, StatusCanceled:
		return true
	default:
		return false
	}
}

// Entitled reports whether the status grants the subscribed plan's entitlements
// (active/trialing/past_due) rather than falling back to Free.
func (s SubStatus) Entitled() bool {
	switch s {
	case StatusActive, StatusTrialing, StatusPastDue:
		return true
	default:
		return false
	}
}

// allowedTransitions encodes the state machine edges of docs/06-BILLING.md §2.
// Self-edges are permitted (idempotent webhook re-application of the same
// status). Stripe is authoritative, so this is a defensive/documentation guard,
// not a gate that can reject a real Stripe state.
var allowedTransitions = map[SubStatus]map[SubStatus]bool{
	StatusNone:     {StatusNone: true, StatusActive: true, StatusTrialing: true},
	StatusTrialing: {StatusTrialing: true, StatusActive: true, StatusPastDue: true, StatusCanceled: true, StatusUnpaid: true},
	StatusActive:   {StatusActive: true, StatusPastDue: true, StatusCanceled: true},
	StatusPastDue:  {StatusPastDue: true, StatusActive: true, StatusUnpaid: true, StatusCanceled: true},
	StatusUnpaid:   {StatusUnpaid: true, StatusActive: true, StatusCanceled: true},
	StatusCanceled: {StatusCanceled: true, StatusActive: true}, // resubscribe reactivates
}

// CanTransition reports whether from→to is a legal edge of the state machine.
// Unknown statuses never transition.
func CanTransition(from, to SubStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	return allowedTransitions[from][to]
}

// ---- Entitlements ---------------------------------------------------------

// Entitlements is the resolved capability envelope for an org, derived from its
// (plan, status) and cached in Redis for 60s (docs/06-BILLING.md §7). Limits use
// -1 for "unlimited".
type Entitlements struct {
	Plan               PlanCode
	Status             SubStatus
	MaxMembers         int
	MaxProjects        int
	MaxStorageBytes    int64
	APIRatePerMin      int
	AuditRetentionDays int
	Metered            bool
	PastDueWarning     bool // status == past_due: keep entitlements but flag the banner
}

// EffectivePlan returns the plan whose limits apply given the status:
// active/trialing/past_due keep the subscribed plan; unpaid/canceled/none fall
// back to Free (docs/06-BILLING.md §2).
func EffectivePlan(subscribed, free Plan, status SubStatus) Plan {
	if status.Entitled() {
		return subscribed
	}
	return free
}

// DeriveEntitlements builds the entitlement envelope from (subscribed plan, free
// plan, status). free is required so a downgraded org resolves Free's real
// limits without the domain hardcoding them (they live in the plans table).
func DeriveEntitlements(subscribed, free Plan, status SubStatus) Entitlements {
	p := EffectivePlan(subscribed, free, status)
	return Entitlements{
		Plan:               p.Code,
		Status:             status,
		MaxMembers:         p.MaxMembers,
		MaxProjects:        p.MaxProjects,
		MaxStorageBytes:    p.MaxStorageBytes,
		APIRatePerMin:      p.APIRatePerMin,
		AuditRetentionDays: p.AuditRetentionDays,
		Metered:            p.Metered,
		PastDueWarning:     status == StatusPastDue,
	}
}

// Allows reports whether current < max (or max is unlimited). Callers pass the
// count that WOULD result including the new resource.
func Unlimited(max int) bool { return max < 0 }

// ---- Models ---------------------------------------------------------------

// Subscription is the local mirror of an org's Stripe subscription (one per
// org). Nil pointers mean "not yet set by Stripe".
type Subscription struct {
	ID                   string
	OrgID                string
	PlanCode             PlanCode
	StripeSubscriptionID *string
	StripeCustomerID     *string
	Status               SubStatus
	CurrentPeriodEnd     *time.Time
	CancelAtPeriodEnd    bool
	LastStripeEventAt    *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Invoice mirrors a Stripe invoice for the history page (FR-BILL-008). Amounts
// are minor units.
type Invoice struct {
	ID              string
	OrgID           string
	StripeInvoiceID string
	Number          string
	Status          string
	AmountDue       int64
	AmountPaid      int64
	Currency        string
	HostedPDFURL    string
	PeriodStart     *time.Time
	PeriodEnd       *time.Time
	CreatedAt       time.Time
}

// UsageMetric names a metered dimension (docs/06-BILLING.md §5).
type UsageMetric string

const (
	MetricActiveMembers UsageMetric = "active_members"
	MetricStorageBytes  UsageMetric = "storage_bytes"
	MetricAPICalls      UsageMetric = "api_calls"
)

// UsageRecord is a per-(org, metric, day) aggregate (FR-BILL-007).
type UsageRecord struct {
	OrgID      string
	Metric     UsageMetric
	PeriodDate time.Time // date at UTC midnight
	Value      int64
	PushedAt   *time.Time
}

// OutboxKind names an outbox row's downstream effect.
type OutboxKind string

const (
	OutboxEmailSend OutboxKind = "email:send"
)

// OutboxItem is a transactional-outbox row (09 §2). Producers insert it in the
// same tx as their write; the drainer hands it to Asynq.
type OutboxItem struct {
	ID        string
	OrgID     string
	Kind      OutboxKind
	Payload   json.RawMessage
	CreatedAt time.Time
	DrainedAt *time.Time
}

// ProcessedEvent is a row in the global webhook idempotency ledger (06 §4).
type ProcessedEvent struct {
	EventID     string
	Type        string
	Payload     json.RawMessage
	Handled     bool
	Error       string
	ProcessedAt time.Time
}
