package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// SubscriptionRepo is the Postgres-backed billing.SubscriptionRepository.
// subscriptions is [T] (RLS), so every method runs through TenantPool.WithTenant.
type SubscriptionRepo struct{ tp *TenantPool }

// NewSubscriptionRepo builds a SubscriptionRepo over the tenant pool.
func NewSubscriptionRepo(tp *TenantPool) *SubscriptionRepo { return &SubscriptionRepo{tp: tp} }

var _ billing.SubscriptionRepository = (*SubscriptionRepo)(nil)

func (r *SubscriptionRepo) Get(ctx context.Context, orgID string) (*billing.Subscription, error) {
	var out *billing.Subscription
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetSubscription(ctx, oid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		s := subFromRow(row)
		out = &s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SubscriptionRepo) Upsert(ctx context.Context, orgID string, s *billing.Subscription) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return upsertSubscription(ctx, q, oid, s)
	})
}

// subFromRow maps a gen.Subscription to the domain model.
func subFromRow(row gen.Subscription) billing.Subscription {
	return billing.Subscription{
		ID:                   row.ID.String(),
		OrgID:                row.OrgID.String(),
		PlanCode:             billing.PlanCode(row.PlanCode),
		StripeSubscriptionID: row.StripeSubscriptionID,
		StripeCustomerID:     row.StripeCustomerID,
		Status:               billing.SubStatus(row.Status),
		CurrentPeriodEnd:     tsPtr(row.CurrentPeriodEnd),
		CancelAtPeriodEnd:    row.CancelAtPeriodEnd,
		LastStripeEventAt:    tsPtr(row.LastStripeEventAt),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

// upsertSubscription writes the full desired subscription state via q (tx-bound
// or pool). last_stripe_event_at is taken from s (the caller owns it — the
// webhook sets it to the event time after its staleness guard).
func upsertSubscription(ctx context.Context, q *gen.Queries, orgUUID uuid.UUID, s *billing.Subscription) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return fmt.Errorf("subscription upsert: id: %w", err)
	}
	return q.UpsertSubscription(ctx, gen.UpsertSubscriptionParams{
		ID:                   id,
		OrgID:                orgUUID,
		PlanCode:             string(s.PlanCode),
		StripeSubscriptionID: s.StripeSubscriptionID,
		StripeCustomerID:     s.StripeCustomerID,
		Status:               string(s.Status),
		CurrentPeriodEnd:     nullTS(s.CurrentPeriodEnd),
		CancelAtPeriodEnd:    s.CancelAtPeriodEnd,
		LastStripeEventAt:    nullTS(s.LastStripeEventAt),
	})
}
