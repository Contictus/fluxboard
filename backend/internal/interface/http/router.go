// Package httpx builds the API's HTTP surface: the chi router, the
// contract-ordered middleware chain, health probes, and route mounting.
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/handlers"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
)

// Deps are the collaborators the router needs. main.go owns construction.
type Deps struct {
	Logger        *slog.Logger
	WebOrigin     string
	Health        Health
	MetricsHTTP   http.Handler
	Auth          *handlers.AuthHandlers
	Orgs          *handlers.OrgHandlers
	Authenticator *mw.Authenticator
	Tenant        *mw.TenantGuard
}

// NewRouter assembles the router. Infrastructure middleware wrap every route;
// the security chain (auth → tenant → rbac → …) wraps only the routes that need
// it, in the order fixed by docs/03-ARCHITECTURE.md §2.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Infrastructure chain (1–4), applied to all routes.
	r.Use(mw.RequestID)
	r.Use(mw.Logger(d.Logger))
	r.Use(mw.Recoverer)
	r.Use(mw.CORS(d.WebOrigin))

	// Operational endpoints (outside auth).
	r.Get("/healthz", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)
	r.Handle("/metrics", d.MetricsHTTP)

	r.Route("/api/v1", func(api chi.Router) {
		// Auth surface (docs/04-AUTH.md §5) — no org context.
		api.Route("/auth", func(a chi.Router) {
			// Public (no access token).
			a.Post("/register", d.Auth.Register)
			a.Post("/login", d.Auth.Login)
			a.Post("/refresh", d.Auth.Refresh)
			a.Post("/2fa/verify", d.Auth.Verify2FA)
			a.Post("/verify-email/request", d.Auth.VerifyEmailRequest)
			a.Post("/verify-email/confirm", d.Auth.VerifyEmailConfirm)
			a.Post("/password/forgot", d.Auth.PasswordForgot)
			a.Post("/password/reset", d.Auth.PasswordReset)
			a.Get("/oauth/google/start", d.Auth.OAuthGoogleStart)
			a.Get("/oauth/google/callback", d.Auth.OAuthGoogleCallback)

			// Authenticated auth endpoints.
			a.Group(func(pr chi.Router) {
				pr.Use(d.Authenticator.Authenticate)
				pr.Post("/logout", d.Auth.Logout)
				pr.Post("/logout-all", d.Auth.LogoutAll)
				pr.Post("/password/change", d.Auth.PasswordChange)
				pr.Get("/sessions", d.Auth.Sessions)
				pr.Delete("/sessions/{id}", d.Auth.RevokeSession)
				pr.Post("/2fa/enroll", d.Auth.Enroll2FA)
				pr.Post("/2fa/activate", d.Auth.Activate2FA)
				pr.Delete("/2fa", d.Auth.Disable2FA)
			})
		})

		// Secured resource surface (Phase 2+). Org-root routes run under auth
		// only (no tenant context yet); org-scoped routes add TenantGuard
		// (resolve membership + RLS scope) and a per-route Casbin gate, in the
		// order fixed by docs/03-ARCHITECTURE.md §2.
		api.Group(func(sec chi.Router) {
			sec.Use(d.Authenticator.Authenticate) // 5

			// Org root (no {orgId} — cannot resolve a tenant).
			sec.Post("/orgs", d.Orgs.CreateOrg)
			sec.Get("/orgs", d.Orgs.ListMyOrgs)
			sec.Post("/invitations/accept", d.Orgs.AcceptInvitation)

			// Org-scoped surface (docs/08 §4).
			sec.Route("/orgs/{orgId}", func(o chi.Router) {
				o.Use(d.Tenant.Resolve) // 6 (+ RLS scope inside usecases)

				read := func(obj string) func(http.Handler) http.Handler {
					return d.Tenant.Require(obj, tenant.ActRead)
				}
				write := func(obj string) func(http.Handler) http.Handler {
					return d.Tenant.Require(obj, tenant.ActWrite)
				}

				o.With(read(tenant.ObjOrg)).Get("/", d.Orgs.GetOrg)
				o.With(write(tenant.ObjOrg)).Patch("/", d.Orgs.UpdateOrg)
				o.With(write(tenant.ObjOwnership)).Delete("/", d.Orgs.DeleteOrg)
				o.With(write(tenant.ObjOwnership)).Post("/restore", d.Orgs.RestoreOrg)
				o.With(write(tenant.ObjOwnership)).Post("/transfer-ownership", d.Orgs.TransferOwnership)

				o.With(read(tenant.ObjOrg)).Get("/members", d.Orgs.ListMembers)
				o.With(read(tenant.ObjOrg)).Delete("/members/me", d.Orgs.Leave)
				o.With(write(tenant.ObjMembers)).Patch("/members/{userId}", d.Orgs.ChangeMemberRole)
				o.With(write(tenant.ObjMembers)).Delete("/members/{userId}", d.Orgs.RemoveMember)

				o.With(write(tenant.ObjInvitations)).Get("/invitations", d.Orgs.ListInvitations)
				o.With(write(tenant.ObjInvitations)).Post("/invitations", d.Orgs.CreateInvitation)
				o.With(write(tenant.ObjInvitations)).Delete("/invitations/{id}", d.Orgs.RevokeInvitation)
				o.With(write(tenant.ObjInvitations)).Post("/invitations/{id}/resend", d.Orgs.ResendInvitation)
			})
		})
	})

	return r
}
