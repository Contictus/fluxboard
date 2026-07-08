package jobs

// This file holds the webhook:retry job (docs/build/PHASE-6 §6, FR-BILL-005 /
// FR-ADM-004). The platform-admin webhook browser enqueues a retry by event id;
// this handler reloads the stored Stripe event and re-runs the idempotent webhook
// consumer. Because ProcessEvent dedups on event id, a retry is only meaningful
// for events recorded handled=false (errored/unroutable); a previously-applied
// event returns Duplicate and is a no-op.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// TypeWebhookRetry is the retry task kind (enqueued by adminuc via the api).
const TypeWebhookRetry = "webhook:retry"

// WebhookRetryPayload is the retry task payload — just the event id to replay.
type WebhookRetryPayload struct {
	EventID string `json:"event_id"`
}

// StoredEventReader reloads a previously-recorded Stripe event by id
// (postgres.ProcessedEventRepo).
type StoredEventReader interface {
	StoredEvent(ctx context.Context, eventID string) (billing.StripeEvent, error)
}

// EventReprocessor re-runs the idempotent webhook consumer (billinguc.Service).
type EventReprocessor interface {
	ProcessEvent(ctx context.Context, ev billing.StripeEvent) (billing.WebhookResult, error)
}

// WebhookRetry handles webhook:retry tasks.
type WebhookRetry struct {
	events StoredEventReader
	proc   EventReprocessor
	log    *slog.Logger
}

// NewWebhookRetry builds the handler.
func NewWebhookRetry(events StoredEventReader, proc EventReprocessor, log *slog.Logger) *WebhookRetry {
	if log == nil {
		log = slog.Default()
	}
	return &WebhookRetry{events: events, proc: proc, log: log}
}

// Register wires the handler onto an Asynq mux.
func (h *WebhookRetry) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeWebhookRetry, h.Handle)
}

// Handle reloads the stored event and reprocesses it.
func (h *WebhookRetry) Handle(ctx context.Context, t *asynq.Task) error {
	var p WebhookRetryPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("webhook:retry: bad payload: %w", err)
	}
	if p.EventID == "" {
		return fmt.Errorf("webhook:retry: empty event_id")
	}
	ev, err := h.events.StoredEvent(ctx, p.EventID)
	if err != nil {
		return fmt.Errorf("webhook:retry: load %s: %w", p.EventID, err)
	}
	res, err := h.proc.ProcessEvent(ctx, ev)
	if err != nil {
		return fmt.Errorf("webhook:retry: reprocess %s: %w", p.EventID, err)
	}
	h.log.Info("webhook retried", "event_id", p.EventID, "duplicate", res.Duplicate)
	return nil
}
