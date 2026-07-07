package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// ---- fakes -------------------------------------------------------------------

type fakeOrgs struct{ ids []string }

func (f *fakeOrgs) ListActiveOrgIDs(context.Context) ([]string, error) { return f.ids, nil }

type fakeOutbox struct {
	items   []billing.OutboxItem
	claimed bool
}

func (f *fakeOutbox) ClaimBatch(_ context.Context, _ string, _ int) ([]billing.OutboxItem, error) {
	if f.claimed {
		return nil, nil
	}
	f.claimed = true
	return f.items, nil
}

type enqueued struct {
	taskType string
	payload  []byte
	taskID   string
	queue    string
}

type fakeEnqueuer struct {
	tasks       []enqueued
	failWith    error
	conflictIDs map[string]bool // TaskIDs to reject as duplicates
}

func (f *fakeEnqueuer) EnqueueContext(_ context.Context, t *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	e := enqueued{taskType: t.Type(), payload: t.Payload()}
	for _, o := range opts {
		switch o.Type() {
		case asynq.TaskIDOpt:
			e.taskID = o.Value().(string)
		case asynq.QueueOpt:
			e.queue = o.Value().(string)
		}
	}
	if f.conflictIDs[e.taskID] {
		return nil, asynq.ErrTaskIDConflict
	}
	if f.failWith != nil {
		return nil, f.failWith
	}
	f.tasks = append(f.tasks, e)
	return &asynq.TaskInfo{}, nil
}

type fakeUsageStore struct {
	upserts []billing.UsageRecord
	forPush []billing.UsageRecord
	pushed  []billing.UsageMetric
	storage int64
}

func (f *fakeUsageStore) Upsert(_ context.Context, _ string, r billing.UsageRecord) error {
	f.upserts = append(f.upserts, r)
	return nil
}
func (f *fakeUsageStore) ListForPush(context.Context, string, time.Time) ([]billing.UsageRecord, error) {
	return f.forPush, nil
}
func (f *fakeUsageStore) MarkPushed(_ context.Context, _ string, m billing.UsageMetric, _, _ time.Time) error {
	f.pushed = append(f.pushed, m)
	return nil
}
func (f *fakeUsageStore) StorageBytes(context.Context, string) (int64, error) { return f.storage, nil }

type fakeCounters struct{ api, members int64 }

func (f *fakeCounters) APICalls(context.Context, string, time.Time) (int64, error) {
	return f.api, nil
}
func (f *fakeCounters) ActiveMembers(context.Context, string, time.Time) (int64, error) {
	return f.members, nil
}

type fakeMailer struct {
	to       []string
	template string
	calls    int
}

func (f *fakeMailer) SendBilling(_ context.Context, to []string, template string) error {
	f.to, f.template = to, template
	f.calls++
	return nil
}

type fakeSubs struct{ sub *billing.Subscription }

func (f *fakeSubs) Get(context.Context, string) (*billing.Subscription, error) {
	if f.sub == nil {
		return nil, domain.ErrNotFound
	}
	return f.sub, nil
}
func (f *fakeSubs) Upsert(_ context.Context, _ string, s *billing.Subscription) error {
	f.sub = s
	return nil
}

type fakePlans struct{ plan billing.Plan }

func (f *fakePlans) Get(context.Context, billing.PlanCode) (*billing.Plan, error) {
	return &f.plan, nil
}
func (f *fakePlans) List(context.Context) ([]billing.Plan, error) { return []billing.Plan{f.plan}, nil }

type fakeGateway struct {
	billing.StripeGateway // panic on anything unstubbed
	remote                *billing.StripeSubscription
	remoteErr             error
	pushed                []billing.UsageMetric
}

func (f *fakeGateway) FetchSubscription(context.Context, string) (*billing.StripeSubscription, error) {
	return f.remote, f.remoteErr
}
func (f *fakeGateway) PushUsage(_ context.Context, _ string, m billing.UsageMetric, _ int64, _ time.Time) error {
	f.pushed = append(f.pushed, m)
	return nil
}

type fakeCache struct{ busted []string }

func (f *fakeCache) Get(context.Context, string) (billing.Entitlements, bool, error) {
	return billing.Entitlements{}, false, nil
}
func (f *fakeCache) Set(context.Context, string, billing.Entitlements, time.Duration) error {
	return nil
}
func (f *fakeCache) Bust(_ context.Context, orgID string) error {
	f.busted = append(f.busted, orgID)
	return nil
}

func str(s string) *string { return &s }

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// ---- outbox:drain / email:send -------------------------------------------------

func TestOutboxDrainEnqueuesWithTaskID(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"template": "billing.dunning", "org_id": "org1"})
	outbox := &fakeOutbox{items: []billing.OutboxItem{
		{ID: "ob1", OrgID: "org1", Kind: billing.OutboxEmailSend, Payload: payload},
	}}
	client := &fakeEnqueuer{}
	b := NewBilling(BillingDeps{
		Outbox: outbox, Client: client,
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	})
	if err := b.HandleOutboxDrain(context.Background(), nil); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(client.tasks) != 1 {
		t.Fatalf("enqueued %d tasks, want 1", len(client.tasks))
	}
	got := client.tasks[0]
	if got.taskType != TypeEmailSend || got.taskID != "outbox:ob1" || got.queue != QueueDefault {
		t.Fatalf("task = %+v, want email:send/outbox:ob1/default", got)
	}
}

func TestOutboxDrainTaskIDConflictIsSuccess(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"template": "billing.dunning", "org_id": "org1"})
	outbox := &fakeOutbox{items: []billing.OutboxItem{
		{ID: "dup", OrgID: "org1", Kind: billing.OutboxEmailSend, Payload: payload},
	}}
	client := &fakeEnqueuer{conflictIDs: map[string]bool{"outbox:dup": true}}
	b := NewBilling(BillingDeps{
		Outbox: outbox, Client: client,
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	})
	if err := b.HandleOutboxDrain(context.Background(), nil); err != nil {
		t.Fatalf("drain with dup TaskID should not error: %v", err)
	}
}

func TestEmailSendDeliversToOwners(t *testing.T) {
	mailer := &fakeMailer{}
	b := NewBilling(BillingDeps{
		Mailer: mailer,
		OwnerEmails: func(context.Context, string) ([]string, error) {
			return []string{"owner@x.test"}, nil
		},
		Logger: discard(),
	})
	payload, _ := json.Marshal(emailPayload{Template: "billing.dunning", OrgID: "org1"})
	if err := b.HandleEmailSend(context.Background(), asynq.NewTask(TypeEmailSend, payload)); err != nil {
		t.Fatalf("email send: %v", err)
	}
	if mailer.template != "billing.dunning" || len(mailer.to) != 1 || mailer.to[0] != "owner@x.test" {
		t.Fatalf("sent %q to %v", mailer.template, mailer.to)
	}
}

func TestEmailSendNoOwnersDropsQuietly(t *testing.T) {
	mailer := &fakeMailer{}
	b := NewBilling(BillingDeps{
		Mailer:      mailer,
		OwnerEmails: func(context.Context, string) ([]string, error) { return nil, nil },
		Logger:      discard(),
	})
	payload, _ := json.Marshal(emailPayload{Template: "billing.welcome", OrgID: "org1"})
	if err := b.HandleEmailSend(context.Background(), asynq.NewTask(TypeEmailSend, payload)); err != nil {
		t.Fatalf("no-owner send should not error: %v", err)
	}
	if mailer.calls != 0 {
		t.Fatal("mailer called despite no recipients")
	}
}

// ---- usage:aggregate / usage:push_stripe ---------------------------------------

func TestUsageAggregateUpsertsAllMetrics(t *testing.T) {
	store := &fakeUsageStore{storage: 4096}
	now := time.Date(2026, 7, 7, 15, 30, 0, 0, time.UTC)
	b := NewBilling(BillingDeps{
		Usage: store, Counters: &fakeCounters{api: 120, members: 4},
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
		Now: func() time.Time { return now },
	})
	if err := b.HandleUsageAggregate(context.Background(), nil); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(store.upserts) != 3 {
		t.Fatalf("upserted %d records, want 3", len(store.upserts))
	}
	want := map[billing.UsageMetric]int64{
		billing.MetricAPICalls:      120,
		billing.MetricActiveMembers: 4,
		billing.MetricStorageBytes:  4096,
	}
	day := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	for _, r := range store.upserts {
		if r.Value != want[r.Metric] {
			t.Errorf("%s = %d, want %d", r.Metric, r.Value, want[r.Metric])
		}
		if !r.PeriodDate.Equal(day) {
			t.Errorf("%s period = %v, want UTC midnight %v", r.Metric, r.PeriodDate, day)
		}
	}
}

func TestUsagePushSkipsUnmeteredAndPushesMetered(t *testing.T) {
	rec := billing.UsageRecord{OrgID: "org1", Metric: billing.MetricAPICalls, Value: 99}
	store := &fakeUsageStore{forPush: []billing.UsageRecord{rec}}
	gw := &fakeGateway{}
	sub := &billing.Subscription{OrgID: "org1", PlanCode: billing.PlanBusiness,
		Status: billing.StatusActive, StripeSubscriptionID: str("sub_1")}
	deps := BillingDeps{
		Usage: store, Subs: &fakeSubs{sub: sub},
		Plans:   &fakePlans{plan: billing.Plan{Code: billing.PlanBusiness, Metered: false}},
		Gateway: gw, Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	}

	// Unmetered plan: nothing pushed.
	if err := NewBilling(deps).HandleUsagePush(context.Background(), nil); err != nil {
		t.Fatalf("push (unmetered): %v", err)
	}
	if len(gw.pushed) != 0 {
		t.Fatalf("pushed %v for unmetered plan", gw.pushed)
	}

	// Metered plan: pushed + marked.
	deps.Plans = &fakePlans{plan: billing.Plan{Code: billing.PlanBusiness, Metered: true}}
	if err := NewBilling(deps).HandleUsagePush(context.Background(), nil); err != nil {
		t.Fatalf("push (metered): %v", err)
	}
	if len(gw.pushed) != 1 || gw.pushed[0] != billing.MetricAPICalls {
		t.Fatalf("pushed = %v, want [api_calls]", gw.pushed)
	}
	if len(store.pushed) != 1 {
		t.Fatalf("marked %d records pushed, want 1", len(store.pushed))
	}
}

// ---- billing:reconcile ---------------------------------------------------------

func TestReconcileHealsDriftAndCounts(t *testing.T) {
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	subs := &fakeSubs{sub: &billing.Subscription{
		OrgID: "org1", PlanCode: billing.PlanPro, Status: billing.StatusActive,
		StripeSubscriptionID: str("sub_1"), CurrentPeriodEnd: &end,
	}}
	gw := &fakeGateway{remote: &billing.StripeSubscription{
		SubscriptionID: "sub_1", Status: billing.StatusPastDue,
		PlanCode: billing.PlanPro, CurrentPeriodEnd: &end,
	}}
	cache := &fakeCache{}
	drift := prometheus.NewCounter(prometheus.CounterOpts{Name: "billing_reconciliation_drift_total"})
	b := NewBilling(BillingDeps{
		Subs: subs, Gateway: gw, Cache: cache, Drift: drift,
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	})
	if err := b.HandleReconcile(context.Background(), nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := testutil.ToFloat64(drift); got != 1 {
		t.Fatalf("drift counter = %v, want 1", got)
	}
	if subs.sub.Status != billing.StatusPastDue {
		t.Fatalf("healed status = %s, want past_due (toward Stripe)", subs.sub.Status)
	}
	if len(cache.busted) != 1 || cache.busted[0] != "org1" {
		t.Fatalf("cache bust = %v, want [org1]", cache.busted)
	}
}

func TestReconcileNoDriftNoHeal(t *testing.T) {
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	local := &billing.Subscription{
		OrgID: "org1", PlanCode: billing.PlanPro, Status: billing.StatusActive,
		StripeSubscriptionID: str("sub_1"), CurrentPeriodEnd: &end, CancelAtPeriodEnd: false,
	}
	subs := &fakeSubs{sub: local}
	gw := &fakeGateway{remote: &billing.StripeSubscription{
		SubscriptionID: "sub_1", Status: billing.StatusActive,
		PlanCode: billing.PlanPro, CurrentPeriodEnd: &end,
	}}
	drift := prometheus.NewCounter(prometheus.CounterOpts{Name: "billing_reconciliation_drift_total"})
	b := NewBilling(BillingDeps{
		Subs: subs, Gateway: gw, Cache: &fakeCache{}, Drift: drift,
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	})
	if err := b.HandleReconcile(context.Background(), nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := testutil.ToFloat64(drift); got != 0 {
		t.Fatalf("drift counter = %v, want 0", got)
	}
}

func TestReconcileStubModeSkips(t *testing.T) {
	subs := &fakeSubs{sub: &billing.Subscription{
		OrgID: "org1", PlanCode: billing.PlanPro, Status: billing.StatusActive,
		StripeSubscriptionID: str("sub_1"),
	}}
	gw := &fakeGateway{remoteErr: billing.ErrReconcileUnsupported}
	drift := prometheus.NewCounter(prometheus.CounterOpts{Name: "billing_reconciliation_drift_total"})
	b := NewBilling(BillingDeps{
		Subs: subs, Gateway: gw, Cache: &fakeCache{}, Drift: drift,
		Orgs: &fakeOrgs{ids: []string{"org1"}}, Logger: discard(),
	})
	if err := b.HandleReconcile(context.Background(), nil); err != nil {
		t.Fatalf("reconcile in stub mode should not error: %v", err)
	}
	if got := testutil.ToFloat64(drift); got != 0 {
		t.Fatalf("drift counter = %v, want 0 in stub mode", got)
	}
}
