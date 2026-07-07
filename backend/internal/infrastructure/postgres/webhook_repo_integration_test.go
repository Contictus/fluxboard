//go:build integration

// Integration tests for the webhook idempotency core (Phase 4 §8.2/§8.3,
// docs/06-BILLING.md §9). They exercise the REAL WebhookRepo.Apply against a
// live Postgres — the unique-violation dedup and the out-of-order staleness
// guard cannot be proven with fakes.
//
// Run with the dockerized stack up and migrated:
//
//	TEST_DATABASE_URL=postgres://... go test -tags=integration ./internal/infrastructure/postgres/
//
// (make test-integration wires this). Skips when TEST_DATABASE_URL is unset.
package postgres

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// newIntegrationDB connects to TEST_DATABASE_URL and seeds the fixtures the
// billing FKs need (an org + the pro plan). Rows are cleaned up per test org.
func newIntegrationDB(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	orgID := uuid.NewString()
	_, err = pool.Exec(ctx,
		`INSERT INTO organizations (id, slug, name) VALUES ($1, $2, $3)`,
		orgID, "it-"+orgID[:8], "webhook integration test")
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO plans (code, name, max_members, max_projects, max_storage_bytes,
		                    api_rate_per_min, audit_retention_days, metered)
		 VALUES ('pro', 'Pro', 25, 50, 53687091200, 300, 30, false)
		 ON CONFLICT (code) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	t.Cleanup(func() {
		// Tenant rows via the GUC (works under app role RLS and owner alike),
		// then the global rows.
		_ = withTenantTx(ctx, pool, orgID, func(tx pgx.Tx) error {
			for _, stmt := range []string{
				`DELETE FROM outbox WHERE org_id = $1`,
				`DELETE FROM subscriptions WHERE org_id = $1`,
			} {
				if _, err := tx.Exec(ctx, stmt, orgID); err != nil {
					return err
				}
			}
			return nil
		})
		_, _ = pool.Exec(ctx, `DELETE FROM processed_stripe_events WHERE payload->>'test_org' = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return pool, orgID
}

// withTenantTx mirrors TenantPool.WithTenant for raw verification SQL.
func withTenantTx(ctx context.Context, pool *pgxpool.Pool, orgID string, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", orgID); err != nil {
			return err
		}
		return fn(tx)
	})
}

// mutation builds a routable subscription-bearing WebhookMutation, tagged with
// the test org so cleanup can find the global ledger rows.
func mutation(orgID, eventID string, created time.Time, status billing.SubStatus) billing.WebhookMutation {
	payload, _ := json.Marshal(map[string]string{"test_org": orgID, "event": eventID})
	subID := "sub_it_" + orgID[:8]
	custID := "cus_it_" + orgID[:8]
	end := created.Add(30 * 24 * time.Hour)
	return billing.WebhookMutation{
		Event: billing.ProcessedEvent{
			EventID: eventID, Type: "customer.subscription.updated",
			Payload: payload, Handled: true,
		},
		EventCreated: created,
		Subscription: &billing.Subscription{
			ID: uuid.NewString(), OrgID: orgID, PlanCode: billing.PlanPro,
			StripeSubscriptionID: &subID, StripeCustomerID: &custID,
			Status: status, CurrentPeriodEnd: &end,
		},
		Outbox: []billing.OutboxItem{{
			ID: uuid.NewString(), OrgID: orgID,
			Kind: billing.OutboxEmailSend, Payload: payload, CreatedAt: created,
		}},
	}
}

// 4.8.2 — replay: the same event applied 5× concurrently produces exactly one
// side-effect set (unique-violation dedup inside one tx). 06 §9.
func TestApplyReplayConcurrentDedup(t *testing.T) {
	pool, orgID := newIntegrationDB(t)
	repo := NewWebhookRepo(NewTenantPool(pool))
	ctx := context.Background()

	eventID := "evt_replay_" + uuid.NewString()
	m := mutation(orgID, eventID, time.Now().UTC().Truncate(time.Second), billing.StatusActive)

	const n = 5
	results := make([]billing.WebhookResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = repo.Apply(ctx, orgID, m)
		}(i)
	}
	wg.Wait()

	applied, dups := 0, 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("apply %d: %v", i, errs[i])
		}
		switch {
		case results[i].Duplicate:
			dups++
		case results[i].SubscriptionCh:
			applied++
		}
	}
	if applied != 1 || dups != n-1 {
		t.Fatalf("applied=%d dups=%d, want 1/%d", applied, dups, n-1)
	}

	var ledger, outbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM processed_stripe_events WHERE event_id = $1`, eventID).Scan(&ledger); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if err := withTenantTx(ctx, pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE org_id = $1`, orgID).Scan(&outbox)
	}); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if ledger != 1 || outbox != 1 {
		t.Fatalf("ledger=%d outbox=%d, want exactly one side-effect set", ledger, outbox)
	}
}

// 4.8.3 — out-of-order: a fresh event carrying an OLDER created time must not
// regress the subscription mirror (staleness guard). 06 §9.
func TestApplyOutOfOrderStaysAtNewest(t *testing.T) {
	pool, orgID := newIntegrationDB(t)
	repo := NewWebhookRepo(NewTenantPool(pool))
	ctx := context.Background()

	t2 := time.Now().UTC().Truncate(time.Second)
	t1 := t2.Add(-1 * time.Hour)

	// Newest state first: active @ T2.
	res, err := repo.Apply(ctx, orgID, mutation(orgID, "evt_ooo_t2_"+uuid.NewString(), t2, billing.StatusActive))
	if err != nil || !res.SubscriptionCh {
		t.Fatalf("apply T2: res=%+v err=%v", res, err)
	}

	// Stale delivery: past_due @ T1 (older). Distinct event id ⇒ not a duplicate.
	res, err = repo.Apply(ctx, orgID, mutation(orgID, "evt_ooo_t1_"+uuid.NewString(), t1, billing.StatusPastDue))
	if err != nil {
		t.Fatalf("apply T1: %v", err)
	}
	if !res.Stale || res.SubscriptionCh {
		t.Fatalf("stale apply res=%+v, want Stale=true SubscriptionCh=false", res)
	}

	var status string
	var lastEvent time.Time
	if err := withTenantTx(ctx, pool, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status, last_stripe_event_at FROM subscriptions WHERE org_id = $1`, orgID).
			Scan(&status, &lastEvent)
	}); err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if status != string(billing.StatusActive) {
		t.Fatalf("mirror status = %s, want active (T2 state preserved)", status)
	}
	if !lastEvent.UTC().Equal(t2) {
		t.Fatalf("last_stripe_event_at = %v, want %v", lastEvent.UTC(), t2)
	}
}
