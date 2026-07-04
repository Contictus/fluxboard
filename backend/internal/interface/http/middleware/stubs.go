package middleware

import "net/http"

// The middleware below are declared now so the chain in router.go reads in its
// final, contract-ordered shape (docs/03-ARCHITECTURE.md §2). Each is a
// pass-through until its owning phase implements it. Wiring them here — rather
// than adding them later — means the security boundary is visible from day one
// and turning a stub real is a localized change.

// Auth parses the bearer token and loads session state (Phase 1).
// TODO(phase1): validate ES256 JWT / API-key, reject revoked sid.
func Auth(next http.Handler) http.Handler { return next }

// TenantResolver resolves the org, verifies membership, and acquires the
// RLS-scoped DB connection (Phase 2, docs/05-TENANCY-RBAC.md §4).
// TODO(phase2): SET LOCAL app.current_tenant; inject TenantContext.
func TenantResolver(next http.Handler) http.Handler { return next }

// RBAC enforces the route-declared object/action via Casbin (Phase 2).
// TODO(phase2): enforcer.Enforce(sub, dom, obj, act).
func RBAC(next http.Handler) http.Handler { return next }

// Entitlement checks plan limits on write routes (Phase 4, docs/06 §7).
// TODO(phase4): consult entitlement cache, 402/403 on limit.
func Entitlement(next http.Handler) http.Handler { return next }

// RateLimit applies a Redis sliding-window limit per plan tier (Phase 4).
// TODO(phase4): key by user/org/API-key, 429 on exceed.
func RateLimit(next http.Handler) http.Handler { return next }
