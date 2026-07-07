package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

type fakeResolver struct{ ent billing.Entitlements }

func (f *fakeResolver) Resolve(context.Context, string) (billing.Entitlements, error) {
	return f.ent, nil
}

type fakeCounter struct {
	allowed  bool
	recorded int
}

func (f *fakeCounter) Allow(context.Context, string, int, time.Time) (bool, error) {
	return f.allowed, nil
}
func (f *fakeCounter) RecordRequest(context.Context, string, string, time.Time) error {
	f.recorded++
	return nil
}

func rlRequest(withTenant bool) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/orgs/org1/projects", nil)
	if withTenant {
		ctx := context.WithValue(r.Context(), ctxKeyTenant, TenantContext{OrgID: "org1", UserID: "u1"})
		r = r.WithContext(ctx)
	}
	return r
}

func runLimit(t *testing.T, rl *RateLimiter, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rl.Limit(next).ServeHTTP(rec, r)
	return rec
}

func TestRateLimitUnderLimitPassesAndRecords(t *testing.T) {
	counter := &fakeCounter{allowed: true}
	rl := &RateLimiter{
		Resolver: &fakeResolver{ent: billing.Entitlements{APIRatePerMin: 60}},
		Counter:  counter, Logger: slog.Default(),
	}
	rec := runLimit(t, rl, rlRequest(true))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if counter.recorded != 1 {
		t.Fatalf("recorded %d requests, want 1", counter.recorded)
	}
}

func TestRateLimitOverLimit429(t *testing.T) {
	counter := &fakeCounter{allowed: false}
	rl := &RateLimiter{
		Resolver: &fakeResolver{ent: billing.Entitlements{APIRatePerMin: 60}},
		Counter:  counter, Logger: slog.Default(),
	}
	rec := runLimit(t, rl, rlRequest(true))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q, want 60", rec.Header().Get("Retry-After"))
	}
	if counter.recorded != 0 {
		t.Fatal("rejected request must not be metered")
	}
}

func TestRateLimitUnlimitedPlanBypassesCheck(t *testing.T) {
	counter := &fakeCounter{allowed: false} // would 429 if consulted
	rl := &RateLimiter{
		Resolver: &fakeResolver{ent: billing.Entitlements{APIRatePerMin: -1}},
		Counter:  counter, Logger: slog.Default(),
	}
	rec := runLimit(t, rl, rlRequest(true))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for unlimited plan", rec.Code)
	}
	if counter.recorded != 1 {
		t.Fatal("unlimited plan must still be metered")
	}
}

func TestRateLimitNoTenantContextPassesThrough(t *testing.T) {
	counter := &fakeCounter{allowed: false}
	rl := &RateLimiter{
		Resolver: &fakeResolver{}, Counter: counter, Logger: slog.Default(),
	}
	rec := runLimit(t, rl, rlRequest(false))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without tenant context", rec.Code)
	}
	if counter.recorded != 0 {
		t.Fatal("no org to meter against")
	}
}
