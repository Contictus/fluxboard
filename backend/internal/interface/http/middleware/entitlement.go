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

// CountFunc returns the org's current usage for a metered resource (e.g. live
// project count, member/seat count). The concrete counts live in the owning
// domains (project/tenant), so main.go supplies these as closures over the
// concrete repos — the middleware package never imports them.
type CountFunc func(ctx context.Context, orgID string) (int64, error)

// EntitlementGuard builds per-route plan-limit gates (FR-BILL-009, docs/06 §7).
// Resolver is required; the CountFuncs are wired for the resources that have a
// gate (nil means "no gate available", which fails closed with 403 rather than
// silently allowing an unbounded resource).
type EntitlementGuard struct {
	Resolver     EntitlementResolver
	ProjectCount CountFunc
	MemberCount  CountFunc
	Logger       *slog.Logger
}

// RequireProjects gates project creation on the plan's max_projects.
func (g *EntitlementGuard) RequireProjects() func(http.Handler) http.Handler {
	return g.gate("max_projects", g.ProjectCount, func(e billing.Entitlements) int { return e.MaxProjects })
}

// RequireMembers gates seat consumption (invite) on the plan's max_members.
func (g *EntitlementGuard) RequireMembers() func(http.Handler) http.Handler {
	return g.gate("max_members", g.MemberCount, func(e billing.Entitlements) int { return e.MaxMembers })
}

// gate builds a middleware that resolves the org's entitlements (cache-first),
// reads the ceiling, and — unless it is unlimited (< 0) — counts current usage
// and returns 402 plan_limit_exceeded with a {limit,current,max} body when the
// request would breach it (current >= max). Runs after TenantGuard.Resolve.
func (g *EntitlementGuard) gate(name string, count CountFunc, ceiling func(billing.Entitlements) int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TenantFrom(r.Context())
			if !ok {
				response.Error(w, domain.ErrForbidden)
				return
			}
			if count == nil { // misconfigured gate: fail closed, never allow.
				g.Logger.Error("entitlement gate has no counter", "limit", name)
				response.Error(w, domain.ErrForbidden)
				return
			}
			ent, err := g.Resolver.Resolve(r.Context(), tc.OrgID)
			if err != nil {
				response.Error(w, err)
				return
			}
			max := ceiling(ent)
			if max < 0 { // unlimited: skip the count entirely.
				next.ServeHTTP(w, r)
				return
			}
			current, err := count(r.Context(), tc.OrgID)
			if err != nil {
				response.Error(w, err)
				return
			}
			if current >= int64(max) {
				response.PlanLimit(w, name, current, int64(max))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
