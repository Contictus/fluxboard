package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// WebhookRepo is the Postgres-backed billing.WebhookRepository — the idempotency
// core (invariant #2, 06 §4). Apply performs the dedup insert, the out-of-order
// staleness guard, and every side effect (subscription/invoice mirror, outbox)
// in ONE tenant-scoped transaction, so "processed at most once, in the same tx
// as its side effects" holds. The global ledger table (processed_stripe_events)
// has no RLS, so it is written inside the tenant tx without issue.
type WebhookRepo struct{ tp *TenantPool }

// NewWebhookRepo builds a WebhookRepo over the tenant pool.
func NewWebhookRepo(tp *TenantPool) *WebhookRepo { return &WebhookRepo{tp: tp} }

var _ billing.WebhookRepository = (*WebhookRepo)(nil)

func (r *WebhookRepo) Apply(ctx context.Context, orgID string, m billing.WebhookMutation) (billing.WebhookResult, error) {
	var res billing.WebhookResult
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)

		// Dedup primitive: rows-affected 0 ⇒ event already processed.
		n, err := q.InsertProcessedEvent(ctx, processedEventParams(m.Event))
		if err != nil {
			return err
		}
		if n == 0 {
			res.Duplicate = true
			return nil // committed ledger unchanged; no side effects
		}

		// Subscription mirror: staleness-gated (out-of-order delivery, 06 §4).
		if m.Subscription != nil {
			stale, err := subscriptionStale(ctx, q, oid, m.EventCreated)
			if err != nil {
				return err
			}
			if stale {
				res.Stale = true
			} else {
				// The webhook owns last_stripe_event_at: stamp it with the event
				// time so a later out-of-order event is recognized as stale.
				sub := *m.Subscription
				et := m.EventCreated
				sub.LastStripeEventAt = &et
				if err := upsertSubscription(ctx, q, oid, &sub); err != nil {
					return err
				}
				res.SubscriptionCh = true
			}
		}

		// Invoice mirror: not staleness-gated (upsert-by-stripe_invoice_id).
		if m.Invoice != nil {
			if err := upsertInvoice(ctx, q, oid, m.Invoice); err != nil {
				return err
			}
		}

		// Outbox rows enqueued in the same tx (drained to Asynq by the worker).
		for i := range m.Outbox {
			if err := insertOutbox(ctx, q, oid, m.Outbox[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return billing.WebhookResult{}, err
	}
	return res, nil
}

// subscriptionStale reports whether an existing subscription row already reflects
// an event at or after eventCreated (so this delivery is out-of-order and its
// subscription mutation must be skipped). No existing row ⇒ never stale.
func subscriptionStale(ctx context.Context, q *gen.Queries, oid uuid.UUID, eventCreated time.Time) (bool, error) {
	last, err := q.GetSubscriptionLastEvent(ctx, oid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if last.Valid && !eventCreated.After(last.Time) {
		return true, nil
	}
	return false, nil
}
