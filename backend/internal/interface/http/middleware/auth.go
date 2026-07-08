package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
)

// ctxKeyPrincipal stores the authenticated Principal. (Offset from the iota keys
// in middleware.go to avoid collision.)
const ctxKeyPrincipal ctxKey = 100

// authCacheTTL matches the usecase session-cache TTL (ADR 008).
const authCacheTTL = 60 * time.Second

// Principal is the authenticated caller derived from a validated access token.
// No role/tenant here — those are resolved per request downstream (ADR 008).
// EmailVerified mirrors the token's `ver` claim for the RequireVerified gate.
type Principal struct {
	UserID        string
	SID           string
	EmailVerified bool
	// ImpersonatedOrg is set (to the target org id) only for a platform-admin
	// impersonation token (FR-ADM-003). When present, the tenant guard grants
	// read-only access to that org and the write-guard rejects any mutation.
	ImpersonatedOrg string
}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKeyPrincipal, p)
}

// PrincipalFrom returns the Principal and whether one is present.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKeyPrincipal).(Principal)
	return p, ok
}

// Authenticator validates access tokens: ES256 signature/claims, then session
// liveness via the cache (Postgres backfill on miss) so revocation is bounded
// to the cache TTL, not token expiry (docs/04-AUTH.md §2).
type Authenticator struct {
	Verifier *jwtx.Verifier
	Cache    auth.SessionCache
	Sessions auth.SessionRepository
	Logger   *slog.Logger
	Now      func() time.Time // injectable for tests; defaults to time.Now
}

// Authenticate is the middleware. On any failure it writes 401 and stops.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	now := a.Now
	if now == nil {
		now = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			response.Error(w, domain.ErrUnauthorized)
			return
		}
		claims, err := a.Verifier.Verify(raw)
		if err != nil {
			response.Error(w, domain.ErrUnauthorized)
			return
		}
		if !a.sessionLive(r.Context(), claims.SID, now()) {
			response.Error(w, domain.ErrUnauthorized)
			return
		}
		ctx := WithPrincipal(r.Context(), Principal{
			UserID:          claims.Subject,
			SID:             claims.SID,
			EmailVerified:   claims.Ver,
			ImpersonatedOrg: claims.Imp,
		})
		ctx = reqmeta.WithActor(ctx, claims.Subject) // enrich audit entries with the actor
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireVerified blocks callers whose email is not verified (FR-AUTH-002). It
// reads the `ver` claim surfaced on the Principal, so it must run AFTER
// Authenticate. Account-management auth routes (logout, sessions, 2FA, resend
// verify) are mounted outside this gate so an unverified user can still act on
// their own account.
func RequireVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			response.Error(w, domain.ErrUnauthorized)
			return
		}
		if !p.EmailVerified {
			response.Error(w, domain.ErrEmailUnverified)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sessionLive checks the cache first and backfills from Postgres on a miss.
func (a *Authenticator) sessionLive(ctx context.Context, sid string, now time.Time) bool {
	active, known, err := a.Cache.Status(ctx, sid)
	if err != nil {
		a.Logger.Warn("session cache read failed; falling back to db", "err", err)
		known = false
	}
	if known {
		return active
	}
	sess, err := a.Sessions.GetByID(ctx, sid)
	if err != nil || !sess.Active(now) {
		return false
	}
	if err := a.Cache.MarkActive(ctx, sid, authCacheTTL); err != nil {
		a.Logger.Warn("session cache backfill failed", "err", err)
	}
	// Advance last_used_at on the backfill path (≤ once per cache TTL). Optional
	// capability: only the concrete repo implements it (FR-AUTH-008).
	if t, ok := a.Sessions.(interface {
		TouchLastUsed(context.Context, string) error
	}); ok {
		if err := t.TouchLastUsed(ctx, sid); err != nil {
			a.Logger.Warn("session touch last_used failed", "err", err)
		}
	}
	return true
}

// bearerToken extracts the token from an "Authorization: Bearer <t>" header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}
