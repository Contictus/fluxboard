package billinguc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// ProcessEvent is the Stripe webhook consumer — the idempotency core (invariant
// #2, 06 §4). The event is already signature-verified and gateway-normalized.
//
// Routable events (non-empty OrgID) go through WebhookRepository.Apply, which
// does the dedup insert, the out-of-order staleness guard, and every side effect
// (subscription/invoice mirror, outbox email) in ONE transaction. Unroutable
// events (no org) are recorded handled=false in the global ledger with no side
// effects. The entitlement cache is busted only when the subscription mirror
// actually moved (result.SubscriptionCh), so at-least-once delivery yields an
// exactly-once effect.
func (s *Service) ProcessEvent(ctx context.Context, ev billing.StripeEvent) (billing.WebhookResult, error) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return billing.WebhookResult{}, fmt.Errorf("marshal event: %w", err)
	}
	now := s.now().UTC()

	// Unroutable: no org to scope a tenant transaction → global ledger only.
	if ev.OrgID == "" {
		inserted, err := s.events.Record(ctx, billing.ProcessedEvent{
			EventID: ev.ID, Type: ev.Type, Payload: payload, Handled: false,
			Error: "unroutable: no org_id", ProcessedAt: now,
		})
		if err != nil {
			return billing.WebhookResult{}, fmt.Errorf("record unroutable event: %w", err)
		}
		return billing.WebhookResult{Duplicate: !inserted}, nil
	}

	m := billing.WebhookMutation{
		Event: billing.ProcessedEvent{
			EventID: ev.ID, Type: ev.Type, Payload: payload, Handled: true, ProcessedAt: now,
		},
		EventCreated: ev.Created,
	}
	handled, err := s.buildSideEffects(ctx, &m, ev, now)
	if err != nil {
		return billing.WebhookResult{}, err
	}
	if !handled {
		m.Event.Handled = false // unknown/unsupported type: record only, no effects
	}

	res, err := s.webhooks.Apply(ctx, ev.OrgID, m)
	if err != nil {
		return billing.WebhookResult{}, fmt.Errorf("apply webhook: %w", err)
	}
	if res.SubscriptionCh {
		s.bust(ctx, ev.OrgID)
	}
	return res, nil
}

// buildSideEffects populates m with the per-type side effects of the 06 §4 table.
// It returns false for event types we don't act on (recorded handled=false).
// Subscription events carry a full StripeSubscription, so the desired mirror is
// built directly; invoice events merge status onto the current mirror.
func (s *Service) buildSideEffects(ctx context.Context, m *billing.WebhookMutation, ev billing.StripeEvent, now time.Time) (bool, error) {
	switch ev.Type {
	case "checkout.session.completed":
		if ev.Sub == nil {
			return false, nil
		}
		m.Subscription = subFromStripe(ev.OrgID, ev.Sub, now)
		m.Outbox = append(m.Outbox, s.email(ev.OrgID, "billing.welcome", now))
		return true, nil

	case "customer.subscription.updated":
		if ev.Sub == nil {
			return false, nil
		}
		m.Subscription = subFromStripe(ev.OrgID, ev.Sub, now)
		return true, nil

	case "customer.subscription.deleted":
		if ev.Sub == nil {
			return false, nil
		}
		sub := subFromStripe(ev.OrgID, ev.Sub, now)
		sub.Status = billing.StatusCanceled
		m.Subscription = sub
		m.Outbox = append(m.Outbox, s.email(ev.OrgID, "billing.canceled", now))
		return true, nil

	case "invoice.finalized":
		if ev.Invoice == nil {
			return false, nil
		}
		m.Invoice = invFromStripe(ev.OrgID, ev.Invoice, now)
		return true, nil

	case "invoice.payment_failed":
		if ev.Invoice == nil {
			return false, nil
		}
		m.Invoice = invFromStripe(ev.OrgID, ev.Invoice, now)
		if err := s.mergeStatus(ctx, m, ev.OrgID, billing.StatusPastDue, now); err != nil {
			return false, err
		}
		m.Outbox = append(m.Outbox, s.email(ev.OrgID, "billing.dunning", now))
		return true, nil

	case "invoice.payment_succeeded":
		if ev.Invoice == nil {
			return false, nil
		}
		m.Invoice = invFromStripe(ev.OrgID, ev.Invoice, now)
		// Only a recovery (past_due → active) changes state + sends an email.
		cur, err := s.currentSub(ctx, ev.OrgID)
		if err != nil {
			return false, err
		}
		if cur != nil && cur.Status == billing.StatusPastDue {
			cur.Status = billing.StatusActive
			cur.UpdatedAt = now
			m.Subscription = cur
			m.Outbox = append(m.Outbox, s.email(ev.OrgID, "billing.resolved", now))
		}
		return true, nil

	default:
		return false, nil
	}
}

// mergeStatus loads the current subscription mirror and, if present, sets a new
// status on the full row so Apply's upsert preserves plan/ids. No local row →
// no subscription mutation (the invoice mirror still records).
func (s *Service) mergeStatus(ctx context.Context, m *billing.WebhookMutation, orgID string, status billing.SubStatus, now time.Time) error {
	cur, err := s.currentSub(ctx, orgID)
	if err != nil {
		return err
	}
	if cur == nil {
		return nil
	}
	cur.Status = status
	cur.UpdatedAt = now
	m.Subscription = cur
	return nil
}

// currentSub returns the org's subscription mirror, or nil on ErrNotFound.
func (s *Service) currentSub(ctx context.Context, orgID string) (*billing.Subscription, error) {
	sub, err := s.subs.Get(ctx, orgID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// email builds an email:send outbox row for a named template (09 §2). It is
// enqueued in the webhook transaction and drained to Asynq by the worker.
func (s *Service) email(orgID, template string, now time.Time) billing.OutboxItem {
	payload, _ := json.Marshal(map[string]string{"template": template, "org_id": orgID})
	return billing.OutboxItem{
		ID: newID(), OrgID: orgID, Kind: billing.OutboxEmailSend, Payload: payload, CreatedAt: now,
	}
}

// subFromStripe maps a gateway-normalized subscription to the full desired mirror
// state. ID/CreatedAt seed the insert path; Apply's ON CONFLICT (org_id) upsert
// preserves them on update.
func subFromStripe(orgID string, ss *billing.StripeSubscription, now time.Time) *billing.Subscription {
	return &billing.Subscription{
		ID:                   newID(),
		OrgID:                orgID,
		PlanCode:             ss.PlanCode,
		StripeSubscriptionID: strPtr(ss.SubscriptionID),
		StripeCustomerID:     strPtr(ss.CustomerID),
		Status:               ss.Status,
		CurrentPeriodEnd:     ss.CurrentPeriodEnd,
		CancelAtPeriodEnd:    ss.CancelAtPeriodEnd,
		LastStripeEventAt:    &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// invFromStripe maps a gateway-normalized invoice to the mirror row (FR-BILL-008).
func invFromStripe(orgID string, si *billing.StripeInvoice, now time.Time) *billing.Invoice {
	return &billing.Invoice{
		ID:              newID(),
		OrgID:           orgID,
		StripeInvoiceID: si.InvoiceID,
		Number:          si.Number,
		Status:          si.Status,
		AmountDue:       si.AmountDue,
		AmountPaid:      si.AmountPaid,
		Currency:        si.Currency,
		HostedPDFURL:    si.HostedPDFURL,
		PeriodStart:     si.PeriodStart,
		PeriodEnd:       si.PeriodEnd,
		CreatedAt:       now,
	}
}
