package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// ctxKeyTenant stores the resolved TenantContext (offset from ctxKeyPrincipal).
const ctxKeyTenant ctxKey = 101

// roleCacheTTL bounds authorization-revocation latency (docs/05 §6, ≤60s).
const roleCacheTTL = 60 * time.Second

// TenantContext is the resolved org scope for a request: which org, which
// caller, and the caller's role in that org.
type TenantContext struct {
	OrgID  string
	UserID string
	Role   tenant.OrgRole
	// Impersonated is true when the caller is a platform admin impersonating this
	// org (FR-ADM-003): role is a synthetic read-only grant, and the write-guard
	// rejects any mutation.
	Impersonated bool
}

// TenantFrom returns the TenantContext and whether one is present.
func TenantFrom(ctx context.Context) (TenantContext, bool) {
	tc, ok := ctx.Value(ctxKeyTenant).(TenantContext)
	return tc, ok
}

// TenantGuard resolves the org + caller role and enforces the coarse org-role
// gate (docs/05-TENANCY-RBAC.md §4 + §3). It sits after Authenticate in the
// chain, so a Principal is already present.
type TenantGuard struct {
	Members tenant.MembershipRepository
	Cache   tenant.MembershipCache
	Authz   tenant.Authorizer
	Logger  *slog.Logger
}

// Resolve reads {orgId} from the path, resolves the caller's role (cache →
// Postgres backfill), and injects TenantContext. A non-member — or a probe for
// an org the caller cannot see — gets 404 (never 403), so cross-tenant object
// references and org existence both stay opaque (docs/05 §6, 08 §1).
func (g *TenantGuard) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			response.Error(w, domain.ErrUnauthorized)
			return
		}
		orgID := chi.URLParam(r, "orgId")
		if orgID == "" {
			response.Error(w, domain.ErrNotFound)
			return
		}
		// API key: bound to one org, no membership. A probe for any other org stays
		// opaque (404). The scope maps to a synthetic role (write⇒ADMIN, read⇒MEMBER)
		// so the existing route gates apply; APIKeyScopeGuard enforces write scope.
		if info, ok := APIKeyFrom(r.Context()); ok {
			if info.OrgID != orgID {
				response.Error(w, domain.ErrNotFound)
				return
			}
			role := tenant.RoleMember
			if info.HasWrite() {
				role = tenant.RoleAdmin
			}
			ctx := context.WithValue(r.Context(), ctxKeyTenant,
				TenantContext{OrgID: orgID, UserID: p.UserID, Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// Impersonation: the token is bound to one org. A probe for any other org
		// stays opaque (404), and the impersonated caller gets a synthetic read-only
		// ADMIN role instead of a membership lookup — the write-guard blocks writes.
		if p.ImpersonatedOrg != "" {
			if p.ImpersonatedOrg != orgID {
				response.Error(w, domain.ErrNotFound)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyTenant,
				TenantContext{OrgID: orgID, UserID: p.UserID, Role: tenant.RoleAdmin, Impersonated: true})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		role, err := g.resolveRole(r.Context(), orgID, p.UserID)
		if err != nil {
			response.Error(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyTenant,
			TenantContext{OrgID: orgID, UserID: p.UserID, Role: role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveRole returns the caller's org role, or domain.ErrNotFound if they are
// not a member.
func (g *TenantGuard) resolveRole(ctx context.Context, orgID, userID string) (tenant.OrgRole, error) {
	if role, known, err := g.Cache.GetRole(ctx, orgID, userID); err != nil {
		g.Logger.Warn("membership cache read failed; falling back to db", "err", err)
	} else if known {
		return role, nil
	}
	m, err := g.Members.Get(ctx, orgID, userID)
	if err != nil {
		return "", err // ErrNotFound → 404
	}
	if err := g.Cache.SetRole(ctx, orgID, userID, m.Role, roleCacheTTL); err != nil {
		g.Logger.Warn("membership cache backfill failed", "err", err)
	}
	return m.Role, nil
}

// ImpersonationReadOnly rejects any mutating request (non GET/HEAD/OPTIONS) made
// under an impersonation token (FR-ADM-003). It runs after Resolve so the
// TenantContext carries the Impersonated flag. Non-impersonated requests pass
// through untouched.
func ImpersonationReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := TenantFrom(r.Context())
		if ok && tc.Impersonated {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// read-only: allowed
			default:
				response.Error(w, domain.ErrForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RejectImpersonationWrite rejects any mutating request (non GET/HEAD/OPTIONS)
// carried by a platform-admin impersonation token, keyed off the Principal's
// `imp` claim rather than a resolved TenantContext. It therefore also guards the
// org-independent secured routes (/me, POST /orgs, invitation accept) that the
// org-subtree-only ImpersonationReadOnly cannot see — an impersonation token must
// never mutate the admin's own account or create orgs (FR-ADM-003). Mount after
// Authenticate. Non-impersonated requests pass through untouched.
func RejectImpersonationWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if ok && p.ImpersonatedOrg != "" {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// read-only: allowed
			default:
				response.Error(w, domain.ErrForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Require enforces that the resolved role may perform (object, action). Deny →
// 403. Mount per route: r.With(guard.Require(tenant.ObjMembers, tenant.ActWrite)).
func (g *TenantGuard) Require(object, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TenantFrom(r.Context())
			if !ok {
				response.Error(w, domain.ErrForbidden)
				return
			}
			allowed, err := g.Authz.Allowed(tc.Role, object, action)
			if err != nil {
				g.Logger.Error("authz check failed", "err", err)
				response.Error(w, domain.ErrForbidden)
				return
			}
			if !allowed {
				response.Error(w, domain.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}
