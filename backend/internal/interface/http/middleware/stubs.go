package middleware

import "net/http"

// The middleware below are declared now so the chain in router.go reads in its
// final, contract-ordered shape (docs/03-ARCHITECTURE.md §2). Each is a
// pass-through until its owning phase implements it. Wiring them here — rather
// than adding them later — means the security boundary is visible from day one
// and turning a stub real is a localized change.

// (Auth is implemented in auth.go as Authenticator.Authenticate; tenant
// resolution + the Casbin org-role gate are implemented in tenant.go as
// TenantGuard.Resolve / TenantGuard.Require.)

// Entitlement checks plan limits on write routes (Phase 4, docs/06 §7).
// TODO(phase4): consult entitlement cache, 402/403 on limit.
func Entitlement(next http.Handler) http.Handler { return next }

// RateLimit applies a Redis sliding-window limit per plan tier (Phase 4).
// TODO(phase4): key by user/org/API-key, 429 on exceed.
func RateLimit(next http.Handler) http.Handler { return next }
