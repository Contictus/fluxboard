package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	stripex "github.com/mesutokul/fluxboard/backend/internal/infrastructure/stripe"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/billinguc"
)

const webhookTestSecret = "whsec_test"

// recordOnlyEvents satisfies the one port an unroutable event touches (the
// global idempotency ledger), letting the 200 path run through a real Service.
type recordOnlyEvents struct{ recorded int }

func (r *recordOnlyEvents) Record(context.Context, billing.ProcessedEvent) (bool, error) {
	r.recorded++
	return true, nil
}

func newWebhookHarness(t *testing.T) (*WebhookHandlers, *recordOnlyEvents) {
	t.Helper()
	gw, err := stripex.New("stub", webhookTestSecret, "http://web.test")
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}
	events := &recordOnlyEvents{}
	svc := billinguc.New(billinguc.Deps{Events: events, Gateway: gw})
	return NewWebhookHandlers(svc, gw, nil), events
}

func postWebhook(h *WebhookHandlers, body, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(body))
	if sig != "" {
		req.Header.Set("X-Stub-Signature", sig)
	}
	rec := httptest.NewRecorder()
	h.Stripe(rec, req)
	return rec
}

// 4.8.4 — bad/absent/tampered signature → 400, never retried (06 §9).
func TestWebhookBadSignature400(t *testing.T) {
	h, events := newWebhookHarness(t)
	body := `{"id":"evt_1","type":"invoice.finalized","created":1751800000,"org_id":""}`

	cases := map[string]string{
		"absent":       "",
		"garbage":      "deadbeef",
		"wrong secret": stripex.Sign("whsec_other", []byte(body)),
	}
	for name, sig := range cases {
		if rec := postWebhook(h, body, sig); rec.Code != http.StatusBadRequest {
			t.Errorf("%s signature: status = %d, want 400", name, rec.Code)
		}
	}

	// Valid signature over DIFFERENT bytes (tampered body after signing).
	sig := stripex.Sign(webhookTestSecret, []byte(body))
	if rec := postWebhook(h, body+" ", sig); rec.Code != http.StatusBadRequest {
		t.Errorf("tampered body: status = %d, want 400", rec.Code)
	}
	if events.recorded != 0 {
		t.Fatalf("rejected events must never reach the ledger; recorded %d", events.recorded)
	}
}

func TestWebhookMalformedPayload400(t *testing.T) {
	h, _ := newWebhookHarness(t)
	body := `{not json`
	rec := postWebhook(h, body, stripex.Sign(webhookTestSecret, []byte(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for malformed payload", rec.Code)
	}
}

func TestWebhookValidSignature200(t *testing.T) {
	h, events := newWebhookHarness(t)
	body := `{"id":"evt_ok","type":"customer.created","created":1751800000,"org_id":""}`
	rec := postWebhook(h, body, stripex.Sign(webhookTestSecret, []byte(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if events.recorded != 1 {
		t.Fatalf("ledger writes = %d, want 1 (unroutable event recorded handled=false)", events.recorded)
	}
}
