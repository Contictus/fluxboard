package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// EntitlementResolver resolves an org's cached entitlement envelope. It is
// implemented by billinguc.Service (Resolve); declaring the port here keeps the
// middleware package off the usecase package (no import cycle, clean layering).
type EntitlementResolver interface {
	Resolve(ctx context.Context, orgID string) (billing.Entitlements, error)
}

// LimitProbe reports current usage and the plan ceiling for the resource a
// request would create, given the org's resolved entitlements and tenant scope.
// name is the ceiling key surfaced in the 402 body (e.g. "max_projects"); a max
// < 0 means unlimited. The concrete probes (project/member/storage counts) are
// wired per route in §6 — the counts live in the owning domains, not billing.
type LimitProbe func(ctx context.Context, e billing.Entitlements, tc TenantContext) (name string, current, max int64, err error)

// EntitlementGuard builds per-route plan-limit gates (FR-BILL-009, docs/06 §7).
type EntitlementGuard struct {
	Resolver EntitlementResolver
	Logger   *slog.Logger
}

// Require builds a middleware that enforces a plan limit before the handler
// runs. It resolves the org's entitlements (cache-first), probes current-vs-max,
// and returns 402 plan_limit_exceeded when creating the resource would breach
// the ceiling. Mount after TenantGuard.Resolve — it needs the TenantContext.
func (g *EntitlementGuard) Require(probe LimitProbe) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TenantFrom(r.Context())
			if !ok {
				response.Error(w, domain.ErrForbidden)
				return
			}
			ent, err := g.Resolver.Resolve(r.Context(), tc.OrgID)
			if err != nil {
				response.Error(w, err)
				return
			}
			name, current, max, err := probe(r.Context(), ent, tc)
			if err != nil {
				response.Error(w, err)
				return
			}
			if max >= 0 && current >= max {
				response.PlanLimit(w, name, current, max)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
