package stripex

import (
	"context"
	"errors"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

const secret = "whsec_stub_test"

func newStub(t *testing.T) *StubGateway {
	t.Helper()
	g, err := New("stub", secret, "https://app.test")
	if err != nil {
		t.Fatal(err)
	}
	return g.(*StubGateway)
}

func TestNewLiveNotImplemented(t *testing.T) {
	if _, err := New("live", secret, ""); err == nil {
		t.Fatal("MODE=live should error until the real adapter lands")
	}
}

func TestConstructEventValid(t *testing.T) {
	g := newStub(t)
	body := []byte(`{"id":"evt_1","type":"checkout.session.completed","created":1720267200,` +
		`"org_id":"org-1","subscription":{"subscription_id":"sub_1","customer_id":"cus_1",` +
		`"status":"active","plan_code":"pro","cancel_at_period_end":false}}`)
	ev, err := g.ConstructEvent(body, Sign(secret, body))
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != "evt_1" || ev.Type != "checkout.session.completed" || ev.OrgID != "org-1" {
		t.Fatalf("event header mismatch: %+v", ev)
	}
	if ev.Created.Unix() != 1720267200 {
		t.Fatalf("created not mapped: %v", ev.Created)
	}
	if ev.Sub == nil || ev.Sub.PlanCode != billing.PlanPro || ev.Sub.Status != billing.StatusActive {
		t.Fatalf("subscription not mapped: %+v", ev.Sub)
	}
}

func TestConstructEventBadSignature(t *testing.T) {
	g := newStub(t)
	body := []byte(`{"id":"evt_2","type":"invoice.finalized","created":1,"org_id":"org-1"}`)
	if _, err := g.ConstructEvent(body, "deadbeef"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("bad signature = %v, want ErrValidation", err)
	}
	// Tampered body under a correct-for-other-body signature also fails.
	if _, err := g.ConstructEvent([]byte(`{"id":"evt_2b"}`), Sign(secret, body)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("tampered body = %v, want ErrValidation", err)
	}
}

func TestConstructEventEmptySecretFailsClosed(t *testing.T) {
	g, _ := New("stub", "", "https://app.test")
	body := []byte(`{"id":"e","type":"t","created":1,"org_id":"o"}`)
	// With no secret configured, even a "correct" empty-key HMAC must be rejected.
	if _, err := g.ConstructEvent(body, Sign("", body)); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty secret should reject: %v", err)
	}
}

func TestCheckoutAndCustomerStub(t *testing.T) {
	g := newStub(t)
	cust, _ := g.EnsureCustomer(context.Background(), "org-1", "")
	if cust != "cus_stub_org-1" {
		t.Fatalf("stub customer = %q", cust)
	}
	sess, _ := g.CreateCheckout(context.Background(), billing.CheckoutParams{OrgID: "org-1", IdempotencyKey: "co:x", CustomerID: cust})
	if sess.CustomerID != cust || sess.URL == "" {
		t.Fatalf("checkout session malformed: %+v", sess)
	}
}
