package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// RequestCounter is the Redis surface the rate limiter needs (implemented by
// redisx.UsageCounter). Declared here to keep the middleware package off the
// infrastructure package, mirroring EntitlementResolver.
type RequestCounter interface {
	// Allow applies the plan's per-minute fixed-window limit (positive limit).
	Allow(ctx context.Context, orgID string, limit int, now time.Time) (bool, error)
	// RecordRequest counts the call for usage metering (api_calls + the daily
	// active-member HyperLogLog, docs/06-BILLING.md §5).
	RecordRequest(ctx context.Context, orgID, userID string, now time.Time) error
}

// RateLimiter enforces the plan-tier API rate limit (FR-BILL-009,
// api_rate_per_min) and records request-path usage counters on every org-scoped
// call. It runs after TenantGuard.Resolve. Redis trouble fails OPEN: metering is
// lossy by design (06 §5) and availability beats a strict limit.
type RateLimiter struct {
	Resolver EntitlementResolver
	Counter  RequestCounter
	Logger   *slog.Logger
}

// Limit is the middleware.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := TenantFrom(r.Context())
		if !ok { // not org-scoped: nothing to key on
			next.ServeHTTP(w, r)
			return
		}
		now := time.Now()
		ent, err := rl.Resolver.Resolve(r.Context(), tc.OrgID)
		if err != nil {
			rl.Logger.Error("rate limit: entitlement resolve failed; allowing", "org", tc.OrgID, "err", err)
			next.ServeHTTP(w, r)
			return
		}
		if limit := ent.APIRatePerMin; limit > 0 {
			allowed, err := rl.Counter.Allow(r.Context(), tc.OrgID, limit, now)
			if err != nil {
				rl.Logger.Error("rate limit: redis check failed; allowing", "org", tc.OrgID, "err", err)
			} else if !allowed {
				response.RateLimited(w, limit)
				return
			}
		}
		if err := rl.Counter.RecordRequest(r.Context(), tc.OrgID, tc.UserID, now); err != nil {
			rl.Logger.Error("usage counter record failed", "org", tc.OrgID, "err", err)
		}
		next.ServeHTTP(w, r)
	})
}
