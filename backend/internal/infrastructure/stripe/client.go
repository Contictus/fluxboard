// Package stripex adapts Stripe to the billing.StripeGateway port. Under
// MODE=stub (docs/build/PHASE-4-BILLING.md §4.0.1) it is a dev fake with no
// external calls: mutations are no-ops and ConstructEvent verifies a crafted
// payload with an HMAC-SHA256 of the body keyed by STRIPE_WEBHOOK_SECRET (the
// shared "stub signature"). The real stripe-go/v78 adapter lands behind the same
// interface later (MODE=live); New returns an error for it until then.
package stripex

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// New builds the StripeGateway for the given mode. "stub" returns the dev fake;
// "live" is not implemented yet (the stripe-go adapter is deferred, §4.0.2).
func New(mode, webhookSecret, baseURL string) (billing.StripeGateway, error) {
	switch mode {
	case "stub", "":
		return &StubGateway{webhookSecret: webhookSecret, baseURL: baseURL}, nil
	case "live":
		return nil, fmt.Errorf("stripex: MODE=live not implemented yet (stub only); wire stripe-go/v78 here")
	default:
		return nil, fmt.Errorf("stripex: unknown STRIPE_MODE %q", mode)
	}
}

// StubGateway is the MODE=stub fake. It fabricates deterministic customer ids and
// URLs, no-ops the mutating calls, and verifies webhook payloads with a shared
// HMAC secret so the webhook path is exercisable end-to-end without Stripe.
type StubGateway struct {
	webhookSecret string
	baseURL       string
}

var _ billing.StripeGateway = (*StubGateway)(nil)

// EnsureCustomer returns the existing id, or a deterministic fake for the org.
func (g *StubGateway) EnsureCustomer(_ context.Context, orgID, existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	return "cus_stub_" + orgID, nil
}

// CreateCheckout returns a fake hosted-Checkout URL carrying the idempotency key.
func (g *StubGateway) CreateCheckout(_ context.Context, p billing.CheckoutParams) (billing.CheckoutSession, error) {
	cust := p.CustomerID
	if cust == "" {
		cust = "cus_stub_" + p.OrgID
	}
	return billing.CheckoutSession{
		URL:        g.baseURL + "/billing/checkout/stub?session=" + p.IdempotencyKey,
		CustomerID: cust,
	}, nil
}

// UpcomingInvoice returns a deterministic stub proration (seat count × $10).
func (g *StubGateway) UpcomingInvoice(_ context.Context, _ string, _ billing.Plan, seats int64) (int64, string, error) {
	return seats * 1000, "usd", nil
}

func (g *StubGateway) UpdateSubscription(context.Context, string, billing.Plan, int64, string) error {
	return nil
}
func (g *StubGateway) CancelSubscription(context.Context, string, bool) error { return nil }
func (g *StubGateway) Resume(context.Context, string) error                   { return nil }

// PortalSession returns a fake Billing Portal URL.
func (g *StubGateway) PortalSession(_ context.Context, _ string, returnURL string) (string, error) {
	return g.baseURL + "/billing/portal/stub?return=" + returnURL, nil
}

func (g *StubGateway) PushUsage(context.Context, string, billing.UsageMetric, int64, time.Time) error {
	return nil
}

// FetchSubscription has no remote to read under MODE=stub; the reconcile job
// treats ErrReconcileUnsupported as "skip org" (nothing to drift from).
func (g *StubGateway) FetchSubscription(context.Context, string) (*billing.StripeSubscription, error) {
	return nil, billing.ErrReconcileUnsupported
}

// stubEvent is the crafted webhook wire format for MODE=stub. It mirrors the
// gateway-normalized billing.StripeEvent so tests can post exactly what the
// consumer expects (the real adapter maps stripe-go's Event to the same shape).
type stubEvent struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Created int64             `json:"created"` // unix seconds
	OrgID   string            `json:"org_id"`
	Sub     *stubSubscription `json:"subscription,omitempty"`
	Invoice *stubInvoice      `json:"invoice,omitempty"`
}

type stubSubscription struct {
	SubscriptionID    string `json:"subscription_id"`
	CustomerID        string `json:"customer_id"`
	Status            string `json:"status"`
	PlanCode          string `json:"plan_code"`
	CurrentPeriodEnd  *int64 `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
}

type stubInvoice struct {
	InvoiceID    string `json:"invoice_id"`
	Number       string `json:"number"`
	Status       string `json:"status"`
	AmountDue    int64  `json:"amount_due"`
	AmountPaid   int64  `json:"amount_paid"`
	Currency     string `json:"currency"`
	HostedPDFURL string `json:"hosted_pdf_url"`
	PeriodStart  *int64 `json:"period_start,omitempty"`
	PeriodEnd    *int64 `json:"period_end,omitempty"`
}

// ConstructEvent verifies the shared-secret HMAC over the raw body and maps the
// crafted payload to a billing.StripeEvent. A bad/absent signature returns
// domain.ErrValidation (the handler answers 400).
func (g *StubGateway) ConstructEvent(payload []byte, sigHeader string) (billing.StripeEvent, error) {
	if !g.validSignature(payload, sigHeader) {
		return billing.StripeEvent{}, domain.ErrValidation
	}
	var se stubEvent
	if err := json.Unmarshal(payload, &se); err != nil {
		return billing.StripeEvent{}, fmt.Errorf("%w: bad stub event json: %v", domain.ErrValidation, err)
	}
	ev := billing.StripeEvent{
		ID:      se.ID,
		Type:    se.Type,
		Created: time.Unix(se.Created, 0).UTC(),
		OrgID:   se.OrgID,
	}
	if se.Sub != nil {
		ev.Sub = &billing.StripeSubscription{
			SubscriptionID:    se.Sub.SubscriptionID,
			CustomerID:        se.Sub.CustomerID,
			Status:            billing.SubStatus(se.Sub.Status),
			PlanCode:          billing.PlanCode(se.Sub.PlanCode),
			CurrentPeriodEnd:  unixPtr(se.Sub.CurrentPeriodEnd),
			CancelAtPeriodEnd: se.Sub.CancelAtPeriodEnd,
		}
	}
	if se.Invoice != nil {
		ev.Invoice = &billing.StripeInvoice{
			InvoiceID:    se.Invoice.InvoiceID,
			Number:       se.Invoice.Number,
			Status:       se.Invoice.Status,
			AmountDue:    se.Invoice.AmountDue,
			AmountPaid:   se.Invoice.AmountPaid,
			Currency:     se.Invoice.Currency,
			HostedPDFURL: se.Invoice.HostedPDFURL,
			PeriodStart:  unixPtr(se.Invoice.PeriodStart),
			PeriodEnd:    unixPtr(se.Invoice.PeriodEnd),
		}
	}
	return ev, nil
}

// validSignature reports whether sigHeader is the hex HMAC-SHA256 of payload
// under the webhook secret. An empty secret rejects everything (fail closed).
func (g *StubGateway) validSignature(payload []byte, sigHeader string) bool {
	if g.webhookSecret == "" || sigHeader == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sigHeader))
}

// Sign returns the stub signature for a body — used by tests and the e2e script
// to craft a valid X-Stub-Signature header.
func Sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func unixPtr(sec *int64) *time.Time {
	if sec == nil {
		return nil
	}
	t := time.Unix(*sec, 0).UTC()
	return &t
}
