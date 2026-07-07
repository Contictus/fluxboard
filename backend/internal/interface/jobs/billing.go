package jobs

// This file holds the Phase 4 §7 billing jobs (docs/06-BILLING.md §5/§8,
// docs/09-REALTIME-JOBS.md §2): outbox drain → email fan-out, the usage
// metering pipeline, and the nightly Stripe reconciliation.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// Task type names (09 §2 job table).
const (
	TypeOutboxDrain      = "outbox:drain"
	TypeEmailSend        = "email:send"
	TypeUsageAggregate   = "usage:aggregate"
	TypeUsagePushStripe  = "usage:push_stripe"
	TypeBillingReconcile = "billing:reconcile"
)

// Queue names + priorities (09 §2): critical:6 default:3 low:1.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// Queues is the worker's queue-priority map (09 §2).
func Queues() map[string]int {
	return map[string]int{QueueCritical: 6, QueueDefault: 3, QueueLow: 1}
}

// outboxDrainBatch caps rows claimed per org per drain tick.
const outboxDrainBatch = 100

// Enqueuer is the slice of asynq.Client the drainer needs (fake-able in tests).
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// UsageStore joins the domain usage port with the concrete storage SUM
// (postgres.UsageRepo satisfies both).
type UsageStore interface {
	billing.UsageRepository
	StorageBytes(ctx context.Context, orgID string) (int64, error)
}

// UsageCounters reads the request-path Redis counters at aggregate time
// (implemented by redisx.UsageCounter).
type UsageCounters interface {
	APICalls(ctx context.Context, orgID string, day time.Time) (int64, error)
	ActiveMembers(ctx context.Context, orgID string, day time.Time) (int64, error)
}

// BillingMailer delivers a billing notification (implemented by mailer.Mailer).
type BillingMailer interface {
	SendBilling(ctx context.Context, to []string, template string) error
}

// BillingDeps wires the billing job handlers. OwnerEmails is a closure over the
// concrete membership repo (same pattern as the entitlement CountFuncs).
type BillingDeps struct {
	Outbox      billing.OutboxRepository
	Usage       UsageStore
	Subs        billing.SubscriptionRepository
	Plans       billing.PlanRepository
	Gateway     billing.StripeGateway
	Cache       billing.EntitlementCache
	Counters    UsageCounters
	Mailer      BillingMailer
	OwnerEmails func(ctx context.Context, orgID string) ([]string, error)
	Client      Enqueuer
	Drift       prometheus.Counter // billing_reconciliation_drift_total
	Orgs        OrgLister
	Logger      *slog.Logger
	Now         func() time.Time // nil ⇒ time.Now (tests override)
}

// Billing runs the Phase 4 billing jobs.
type Billing struct{ d BillingDeps }

// NewBilling builds the billing job handlers.
func NewBilling(d BillingDeps) *Billing {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Billing{d: d}
}

// Register wires the handlers onto an Asynq mux.
func (b *Billing) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeOutboxDrain, b.HandleOutboxDrain)
	mux.HandleFunc(TypeEmailSend, b.HandleEmailSend)
	mux.HandleFunc(TypeUsageAggregate, b.HandleUsageAggregate)
	mux.HandleFunc(TypeUsagePushStripe, b.HandleUsagePush)
	mux.HandleFunc(TypeBillingReconcile, b.HandleReconcile)
}

// ---- outbox:drain + email:send ---------------------------------------------

// emailPayload is the outbox email:send payload shape produced by the webhook
// consumer (billinguc/webhook.go email()).
type emailPayload struct {
	Template string `json:"template"`
	OrgID    string `json:"org_id"`
}

// HandleOutboxDrain claims undrained outbox rows per org and hands each to
// Asynq with TaskID = the outbox row id: the claim is at-least-once, the
// TaskID makes the enqueue at-most-once, and idempotent handlers give
// exactly-once effect (ADR-011).
func (b *Billing) HandleOutboxDrain(ctx context.Context, _ *asynq.Task) error {
	return forEachOrg(ctx, b.d.Orgs, b.d.Logger, TypeOutboxDrain, func(orgID string) (int, error) {
		items, err := b.d.Outbox.ClaimBatch(ctx, orgID, outboxDrainBatch)
		if err != nil {
			return 0, err
		}
		n := 0
		for _, item := range items {
			if err := b.enqueueOutboxItem(ctx, item); err != nil {
				// The row is already drained; a lost enqueue is logged loudly
				// rather than failing the whole batch (surfaced in admin later).
				b.d.Logger.Error("outbox: enqueue failed", "outbox_id", item.ID, "kind", item.Kind, "err", err)
				continue
			}
			n++
		}
		return n, nil
	})
}

func (b *Billing) enqueueOutboxItem(ctx context.Context, item billing.OutboxItem) error {
	var taskType string
	switch item.Kind {
	case billing.OutboxEmailSend:
		taskType = TypeEmailSend
	default:
		return fmt.Errorf("unknown outbox kind %q", item.Kind)
	}
	_, err := b.d.Client.EnqueueContext(ctx, asynq.NewTask(taskType, item.Payload),
		asynq.TaskID("outbox:"+item.ID), asynq.Queue(QueueDefault), asynq.MaxRetry(5))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // duplicate drain of the same row: dedup did its job
	}
	return err
}

// HandleEmailSend delivers one billing notification to the org's OWNERs.
func (b *Billing) HandleEmailSend(ctx context.Context, t *asynq.Task) error {
	var p emailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("email:send: bad payload: %w", err)
	}
	to, err := b.d.OwnerEmails(ctx, p.OrgID)
	if err != nil {
		return fmt.Errorf("email:send: owner lookup: %w", err)
	}
	if len(to) == 0 {
		b.d.Logger.Warn("email:send: org has no owners; dropping", "org", p.OrgID, "template", p.Template)
		return nil
	}
	return b.d.Mailer.SendBilling(ctx, to, p.Template)
}

// ---- usage:aggregate + usage:push_stripe ------------------------------------

// utcDay truncates t to its UTC midnight (usage_records.period_date grain).
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// HandleUsageAggregate snapshots today's Redis counters (+ the Postgres storage
// SUM) into usage_records. Set-semantics upsert ⇒ re-runnable (FR-BILL-007).
func (b *Billing) HandleUsageAggregate(ctx context.Context, _ *asynq.Task) error {
	day := utcDay(b.d.Now())
	return forEachOrg(ctx, b.d.Orgs, b.d.Logger, TypeUsageAggregate, func(orgID string) (int, error) {
		api, err := b.d.Counters.APICalls(ctx, orgID, day)
		if err != nil {
			return 0, err
		}
		members, err := b.d.Counters.ActiveMembers(ctx, orgID, day)
		if err != nil {
			return 0, err
		}
		storage, err := b.d.Usage.StorageBytes(ctx, orgID)
		if err != nil {
			return 0, err
		}
		for _, rec := range []billing.UsageRecord{
			{OrgID: orgID, Metric: billing.MetricAPICalls, PeriodDate: day, Value: api},
			{OrgID: orgID, Metric: billing.MetricActiveMembers, PeriodDate: day, Value: members},
			{OrgID: orgID, Metric: billing.MetricStorageBytes, PeriodDate: day, Value: storage},
		} {
			if err := b.d.Usage.Upsert(ctx, orgID, rec); err != nil {
				return 0, err
			}
		}
		return 3, nil
	})
}

// HandleUsagePush reports yesterday's aggregates to Stripe with action=set for
// orgs on a metered plan with a live subscription. pushed_at IS NULL filtering
// + set semantics make re-runs harmless (FR-BILL-007).
func (b *Billing) HandleUsagePush(ctx context.Context, _ *asynq.Task) error {
	now := b.d.Now()
	day := utcDay(now.Add(-24 * time.Hour))
	return forEachOrg(ctx, b.d.Orgs, b.d.Logger, TypeUsagePushStripe, func(orgID string) (int, error) {
		sub, err := b.d.Subs.Get(ctx, orgID)
		if errors.Is(err, domain.ErrNotFound) {
			return 0, nil // free org: nothing metered
		}
		if err != nil {
			return 0, err
		}
		if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
			return 0, nil
		}
		plan, err := b.d.Plans.Get(ctx, sub.PlanCode)
		if err != nil {
			return 0, err
		}
		if !plan.Metered {
			return 0, nil
		}
		recs, err := b.d.Usage.ListForPush(ctx, orgID, day)
		if err != nil {
			return 0, err
		}
		n := 0
		for _, rec := range recs {
			if err := b.d.Gateway.PushUsage(ctx, *sub.StripeSubscriptionID, rec.Metric, rec.Value, rec.PeriodDate); err != nil {
				return n, err
			}
			if err := b.d.Usage.MarkPushed(ctx, orgID, rec.Metric, rec.PeriodDate, now); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	})
}

// ---- billing:reconcile -------------------------------------------------------

// HandleReconcile diffs each org's local subscription mirror against Stripe and
// heals toward Stripe on drift (Stripe is the source of truth — 06 §8, ADR-010).
// Every drift increments billing_reconciliation_drift_total and logs at ERROR.
func (b *Billing) HandleReconcile(ctx context.Context, _ *asynq.Task) error {
	return forEachOrg(ctx, b.d.Orgs, b.d.Logger, TypeBillingReconcile, func(orgID string) (int, error) {
		sub, err := b.d.Subs.Get(ctx, orgID)
		if errors.Is(err, domain.ErrNotFound) {
			return 0, nil // never subscribed: nothing to reconcile
		}
		if err != nil {
			return 0, err
		}
		if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
			return 0, nil
		}
		remote, err := b.d.Gateway.FetchSubscription(ctx, *sub.StripeSubscriptionID)
		if errors.Is(err, billing.ErrReconcileUnsupported) {
			b.d.Logger.Debug("reconcile: gateway has no remote (stub mode); skipping", "org", orgID)
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if !subDrifted(sub, remote) {
			return 0, nil
		}
		b.d.Drift.Inc()
		b.d.Logger.Error("reconcile: local mirror drifted from Stripe; healing",
			"org", orgID, "local_status", sub.Status, "remote_status", remote.Status,
			"local_plan", sub.PlanCode, "remote_plan", remote.PlanCode)
		healed := *sub
		healed.Status = remote.Status
		healed.PlanCode = remote.PlanCode
		healed.CurrentPeriodEnd = remote.CurrentPeriodEnd
		healed.CancelAtPeriodEnd = remote.CancelAtPeriodEnd
		if err := b.d.Subs.Upsert(ctx, orgID, &healed); err != nil {
			return 0, err
		}
		if err := b.d.Cache.Bust(ctx, orgID); err != nil {
			b.d.Logger.Error("reconcile: cache bust failed", "org", orgID, "err", err)
		}
		return 1, nil
	})
}

// subDrifted compares the reconcile-relevant fields (status, plan, period end,
// scheduled cancel) between the local mirror and Stripe's view (06 §8).
func subDrifted(local *billing.Subscription, remote *billing.StripeSubscription) bool {
	if local.Status != remote.Status || local.PlanCode != remote.PlanCode ||
		local.CancelAtPeriodEnd != remote.CancelAtPeriodEnd {
		return true
	}
	switch {
	case local.CurrentPeriodEnd == nil && remote.CurrentPeriodEnd == nil:
		return false
	case local.CurrentPeriodEnd == nil || remote.CurrentPeriodEnd == nil:
		return true
	default:
		return !local.CurrentPeriodEnd.Equal(*remote.CurrentPeriodEnd)
	}
}
