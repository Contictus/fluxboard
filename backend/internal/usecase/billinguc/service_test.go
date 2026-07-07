package billinguc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// ---- fakes ----------------------------------------------------------------

var (
	freePlan = billing.Plan{Code: billing.PlanFree, MaxMembers: 5, MaxProjects: 3, MaxStorageBytes: 2 << 30, APIRatePerMin: 60, AuditRetentionDays: 7}
	proPlan  = billing.Plan{Code: billing.PlanPro, MaxMembers: 25, MaxProjects: 50, MaxStorageBytes: 50 << 30, APIRatePerMin: 300, AuditRetentionDays: 30}
)

type fakePlans struct {
	m map[billing.PlanCode]billing.Plan
}

func (f *fakePlans) Get(_ context.Context, c billing.PlanCode) (*billing.Plan, error) {
	if p, ok := f.m[c]; ok {
		return &p, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakePlans) List(context.Context) ([]billing.Plan, error) { return nil, nil }

type fakeSubs struct {
	m       map[string]*billing.Subscription
	upserts int
}

func (f *fakeSubs) Get(_ context.Context, orgID string) (*billing.Subscription, error) {
	if s, ok := f.m[orgID]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeSubs) Upsert(_ context.Context, orgID string, s *billing.Subscription) error {
	f.upserts++
	cp := *s
	f.m[orgID] = &cp
	return nil
}

type fakeInvoices struct{ m map[string][]billing.Invoice }

func (f *fakeInvoices) Upsert(_ context.Context, orgID string, inv *billing.Invoice) error {
	f.m[orgID] = append(f.m[orgID], *inv)
	return nil
}
func (f *fakeInvoices) ListByOrg(_ context.Context, orgID string) ([]billing.Invoice, error) {
	return f.m[orgID], nil
}

type fakeEvents struct {
	seen     map[string]bool
	recorded []billing.ProcessedEvent
}

func (f *fakeEvents) Record(_ context.Context, e billing.ProcessedEvent) (bool, error) {
	f.recorded = append(f.recorded, e)
	if f.seen[e.EventID] {
		return false, nil
	}
	f.seen[e.EventID] = true
	return true, nil
}

type fakeWebhooks struct {
	applied []billing.WebhookMutation
	result  billing.WebhookResult
}

func (f *fakeWebhooks) Apply(_ context.Context, _ string, m billing.WebhookMutation) (billing.WebhookResult, error) {
	f.applied = append(f.applied, m)
	res := f.result
	res.SubscriptionCh = m.Subscription != nil // mimic infra: mirror moved iff a sub was written
	return res, nil
}

type fakeGateway struct {
	checkoutParams billing.CheckoutParams
	updatedPlan    billing.Plan
	updateIdem     string
}

func (f *fakeGateway) EnsureCustomer(_ context.Context, _, existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	return "cus_new", nil
}
func (f *fakeGateway) CreateCheckout(_ context.Context, p billing.CheckoutParams) (billing.CheckoutSession, error) {
	f.checkoutParams = p
	return billing.CheckoutSession{URL: "https://checkout/" + p.IdempotencyKey, CustomerID: p.CustomerID}, nil
}
func (f *fakeGateway) UpcomingInvoice(context.Context, string, billing.Plan, int64) (int64, string, error) {
	return 1234, "usd", nil
}
func (f *fakeGateway) UpdateSubscription(_ context.Context, _ string, p billing.Plan, _ int64, idem string) error {
	f.updatedPlan, f.updateIdem = p, idem
	return nil
}
func (f *fakeGateway) CancelSubscription(context.Context, string, bool) error { return nil }
func (f *fakeGateway) Resume(context.Context, string) error                   { return nil }
func (f *fakeGateway) PortalSession(context.Context, string, string) (string, error) {
	return "https://portal", nil
}
func (f *fakeGateway) PushUsage(context.Context, string, billing.UsageMetric, int64, time.Time) error {
	return nil
}
func (f *fakeGateway) ConstructEvent([]byte, string) (billing.StripeEvent, error) {
	return billing.StripeEvent{}, nil
}
func (f *fakeGateway) FetchSubscription(context.Context, string) (*billing.StripeSubscription, error) {
	return nil, billing.ErrReconcileUnsupported
}

type fakeCache struct {
	m     map[string]billing.Entitlements
	sets  int
	busts int
}

func (f *fakeCache) Get(_ context.Context, orgID string) (billing.Entitlements, bool, error) {
	e, ok := f.m[orgID]
	return e, ok, nil
}
func (f *fakeCache) Set(_ context.Context, orgID string, e billing.Entitlements, _ time.Duration) error {
	f.sets++
	f.m[orgID] = e
	return nil
}
func (f *fakeCache) Bust(_ context.Context, orgID string) error {
	f.busts++
	delete(f.m, orgID)
	return nil
}

// harness bundles a Service with its fakes for assertions.
type harness struct {
	svc      *Service
	plans    *fakePlans
	subs     *fakeSubs
	invoices *fakeInvoices
	events   *fakeEvents
	webhooks *fakeWebhooks
	gateway  *fakeGateway
	cache    *fakeCache
}

func newHarness() *harness {
	h := &harness{
		plans:    &fakePlans{m: map[billing.PlanCode]billing.Plan{billing.PlanFree: freePlan, billing.PlanPro: proPlan}},
		subs:     &fakeSubs{m: map[string]*billing.Subscription{}},
		invoices: &fakeInvoices{m: map[string][]billing.Invoice{}},
		events:   &fakeEvents{seen: map[string]bool{}},
		webhooks: &fakeWebhooks{},
		gateway:  &fakeGateway{},
		cache:    &fakeCache{m: map[string]billing.Entitlements{}},
	}
	fixed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	h.svc = New(Deps{
		Plans: h.plans, Subs: h.subs, Invoices: h.invoices, Events: h.events,
		Webhooks: h.webhooks, Gateway: h.gateway, Cache: h.cache,
		Now: func() time.Time { return fixed }, BaseURL: "https://app.test",
	})
	return h
}

// ---- Resolve --------------------------------------------------------------

func TestResolveCacheHit(t *testing.T) {
	h := newHarness()
	want := billing.Entitlements{Plan: billing.PlanPro, MaxProjects: 50}
	h.cache.m["org1"] = want
	got, err := h.svc.Resolve(context.Background(), "org1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan != billing.PlanPro || h.cache.sets != 0 {
		t.Fatalf("cache hit should not re-derive: got %+v sets=%d", got, h.cache.sets)
	}
}

func TestResolveMissDerivesAndCaches(t *testing.T) {
	h := newHarness()
	// no subscription row → Free/none.
	got, err := h.svc.Resolve(context.Background(), "org1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan != billing.PlanFree || got.MaxProjects != 3 {
		t.Fatalf("no-sub org should resolve Free: %+v", got)
	}
	if h.cache.sets != 1 {
		t.Fatalf("miss should cache once, got sets=%d", h.cache.sets)
	}
	// pro active subscription → pro limits.
	h.subs.m["org2"] = &billing.Subscription{OrgID: "org2", PlanCode: billing.PlanPro, Status: billing.StatusActive}
	got, _ = h.svc.Resolve(context.Background(), "org2")
	if got.Plan != billing.PlanPro || got.MaxProjects != 50 {
		t.Fatalf("pro active should resolve Pro: %+v", got)
	}
}

// ---- Checkout -------------------------------------------------------------

func TestCheckoutIdempotencyKeyAndCustomer(t *testing.T) {
	h := newHarness()
	sess, err := h.svc.Checkout(context.Background(), "org1", billing.PlanPro, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantKey := "co:org1:pro:" + itoa(time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC).Unix()/300)
	if h.gateway.checkoutParams.IdempotencyKey != wantKey {
		t.Fatalf("idem key = %q, want %q", h.gateway.checkoutParams.IdempotencyKey, wantKey)
	}
	if sess.CustomerID != "cus_new" || h.subs.upserts != 1 {
		t.Fatalf("lazy customer not persisted: cust=%s upserts=%d", sess.CustomerID, h.subs.upserts)
	}
	if h.subs.m["org1"].StripeCustomerID == nil || *h.subs.m["org1"].StripeCustomerID != "cus_new" {
		t.Fatal("customer id not stored on subscription row")
	}
}

func TestCheckoutRejectsFreePlan(t *testing.T) {
	h := newHarness()
	if _, err := h.svc.Checkout(context.Background(), "org1", billing.PlanFree, 1); err != domain.ErrValidation {
		t.Fatalf("free plan checkout = %v, want ErrValidation", err)
	}
}

// ---- ApplyChange ----------------------------------------------------------

func TestApplyChangeBustsCache(t *testing.T) {
	h := newHarness()
	subID := "sub_1"
	h.subs.m["org1"] = &billing.Subscription{OrgID: "org1", PlanCode: billing.PlanFree, Status: billing.StatusActive, StripeSubscriptionID: &subID}
	if err := h.svc.ApplyChange(context.Background(), "org1", billing.PlanPro); err != nil {
		t.Fatal(err)
	}
	if h.gateway.updatedPlan.Code != billing.PlanPro || !strings.HasPrefix(h.gateway.updateIdem, "chg:org1:pro:") {
		t.Fatalf("update not called correctly: plan=%s idem=%s", h.gateway.updatedPlan.Code, h.gateway.updateIdem)
	}
	if h.cache.busts != 1 {
		t.Fatalf("apply should bust cache once, got %d", h.cache.busts)
	}
}

func TestApplyChangeNoSubscription(t *testing.T) {
	h := newHarness()
	if err := h.svc.ApplyChange(context.Background(), "org1", billing.PlanPro); err != domain.ErrNotFound {
		t.Fatalf("no sub row = %v, want ErrNotFound", err)
	}
}

// ---- ProcessEvent ---------------------------------------------------------

func TestProcessEventUnroutable(t *testing.T) {
	h := newHarness()
	res, err := h.svc.ProcessEvent(context.Background(), billing.StripeEvent{ID: "evt_x", Type: "customer.subscription.updated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.webhooks.applied) != 0 {
		t.Fatal("unroutable event must not call Apply")
	}
	if len(h.events.recorded) != 1 || h.events.recorded[0].Handled {
		t.Fatal("unroutable event should record handled=false in global ledger")
	}
	if res.Duplicate {
		t.Fatal("first delivery not a duplicate")
	}
	// second delivery of same id → duplicate.
	res, _ = h.svc.ProcessEvent(context.Background(), billing.StripeEvent{ID: "evt_x", Type: "customer.subscription.updated"})
	if !res.Duplicate {
		t.Fatal("replay of unroutable event should be duplicate")
	}
}

func TestProcessEventCheckoutCompleted(t *testing.T) {
	h := newHarness()
	end := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	ev := billing.StripeEvent{
		ID: "evt_1", Type: "checkout.session.completed", OrgID: "org1",
		Created: time.Now(),
		Sub: &billing.StripeSubscription{
			SubscriptionID: "sub_1", CustomerID: "cus_1", Status: billing.StatusActive,
			PlanCode: billing.PlanPro, CurrentPeriodEnd: &end,
		},
	}
	res, err := h.svc.ProcessEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.webhooks.applied) != 1 {
		t.Fatal("expected one Apply")
	}
	m := h.webhooks.applied[0]
	if m.Subscription == nil || m.Subscription.Status != billing.StatusActive || m.Subscription.PlanCode != billing.PlanPro {
		t.Fatalf("subscription mutation wrong: %+v", m.Subscription)
	}
	if len(m.Outbox) != 1 || m.Outbox[0].Kind != billing.OutboxEmailSend {
		t.Fatal("expected one welcome outbox email")
	}
	if !m.Event.Handled {
		t.Fatal("handled event should record handled=true")
	}
	if !res.SubscriptionCh || h.cache.busts != 1 {
		t.Fatalf("subscription change should bust cache: chg=%v busts=%d", res.SubscriptionCh, h.cache.busts)
	}
}

func TestProcessEventPaymentFailedPastDue(t *testing.T) {
	h := newHarness()
	subID := "sub_1"
	h.subs.m["org1"] = &billing.Subscription{OrgID: "org1", PlanCode: billing.PlanPro, Status: billing.StatusActive, StripeSubscriptionID: &subID}
	ev := billing.StripeEvent{
		ID: "evt_2", Type: "invoice.payment_failed", OrgID: "org1", Created: time.Now(),
		Invoice: &billing.StripeInvoice{InvoiceID: "in_1", Number: "F-1", Status: "open", AmountDue: 5000, Currency: "usd"},
	}
	if _, err := h.svc.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	m := h.webhooks.applied[0]
	if m.Invoice == nil || m.Invoice.AmountDue != 5000 {
		t.Fatalf("invoice mirror missing: %+v", m.Invoice)
	}
	if m.Subscription == nil || m.Subscription.Status != billing.StatusPastDue || m.Subscription.PlanCode != billing.PlanPro {
		t.Fatalf("status should merge to past_due preserving plan: %+v", m.Subscription)
	}
	if len(m.Outbox) != 1 {
		t.Fatal("expected dunning email")
	}
}

func TestProcessEventUnknownType(t *testing.T) {
	h := newHarness()
	ev := billing.StripeEvent{ID: "evt_3", Type: "customer.updated", OrgID: "org1", Created: time.Now()}
	if _, err := h.svc.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	m := h.webhooks.applied[0]
	if m.Event.Handled {
		t.Fatal("unknown type should record handled=false")
	}
	if m.Subscription != nil || m.Invoice != nil || len(m.Outbox) != 0 {
		t.Fatal("unknown type should have no side effects")
	}
	if h.cache.busts != 0 {
		t.Fatal("no side effects → no cache bust")
	}
}

// itoa avoids importing strconv just for the key assertion.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
