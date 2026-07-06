// This file is the Stripe webhook ingress (FR-BILL-005). It is the one API
// surface mounted OUTSIDE session auth — Stripe carries no bearer token — so the
// handler authenticates the request itself by verifying the payload signature
// through the gateway before doing any work. See docs/06-BILLING.md §4.
package handlers

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/billinguc"
)

// stubSignatureHeader carries the MODE=stub shared-secret HMAC of the raw body;
// "Stripe-Signature" is the live header. Which one is authoritative is the
// gateway's concern (ConstructEvent) — the handler just forwards whichever is set.
const (
	stubSignatureHeader = "X-Stub-Signature"
	liveSignatureHeader = "Stripe-Signature"
)

// maxWebhookBody caps the raw body. Stripe events are small; this guards abuse.
const maxWebhookBody = 1 << 20 // 1 MiB

// WebhookHandlers serves the unauthenticated Stripe webhook endpoint.
type WebhookHandlers struct {
	svc     *billinguc.Service
	gateway billing.StripeGateway
	logger  *slog.Logger
}

// NewWebhookHandlers builds WebhookHandlers.
func NewWebhookHandlers(svc *billinguc.Service, gateway billing.StripeGateway, logger *slog.Logger) *WebhookHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookHandlers{svc: svc, gateway: gateway, logger: logger}
}

// Stripe consumes a webhook event (FR-BILL-005). It reads the raw body (the
// signature is over the exact bytes, so no decode before verify), verifies it
// through the gateway, then hands the normalized event to ProcessEvent. Status
// contract, chosen so Stripe's retry policy does the right thing:
//   - 200 on commit OR duplicate/stale — the event is durably accounted for, so
//     Stripe must stop retrying.
//   - 400 on a bad/absent signature or malformed payload — never retryable.
//   - 500 on a processing error — Stripe retries with backoff.
func (h *WebhookHandlers) Stripe(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		h.logger.WarnContext(r.Context(), "webhook body read failed", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sig := r.Header.Get(stubSignatureHeader)
	if sig == "" {
		sig = r.Header.Get(liveSignatureHeader)
	}

	ev, err := h.gateway.ConstructEvent(body, sig)
	if err != nil {
		// Bad/absent signature or unparseable payload → 400, never retried.
		h.logger.WarnContext(r.Context(), "webhook signature/parse rejected", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.svc.ProcessEvent(r.Context(), ev)
	if err != nil {
		// Transient/processing failure → 500 so Stripe retries.
		h.logger.ErrorContext(r.Context(), "webhook processing failed",
			"event", ev.ID, "type", ev.Type, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.logger.InfoContext(r.Context(), "webhook processed",
		"event", ev.ID, "type", ev.Type,
		"duplicate", res.Duplicate, "stale", res.Stale, "sub_changed", res.SubscriptionCh)
	w.WriteHeader(http.StatusOK)
}
