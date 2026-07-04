// Package httpx builds the API's HTTP surface: the chi router, the
// contract-ordered middleware chain, health probes, and route mounting.
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

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
	Authenticator *mw.Authenticator
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
			a.Post("/verify-email/confirm", d.Auth.VerifyEmailConfirm)

			// Authenticated auth endpoints.
			a.Group(func(pr chi.Router) {
				pr.Use(d.Authenticator.Authenticate)
				pr.Post("/logout", d.Auth.Logout)
				pr.Post("/logout-all", d.Auth.LogoutAll)
			})
		})

		// Secured resource surface (Phase 2+): the full security chain in
		// contract order. Handlers mount here as later phases land.
		api.Group(func(sec chi.Router) {
			sec.Use(d.Authenticator.Authenticate) // 5
			sec.Use(mw.TenantResolver)            // 6
			sec.Use(mw.RBAC)                      // 7
			sec.Use(mw.Entitlement)               // 8
			sec.Use(mw.RateLimit)                 // 9
			// TODO(phase2+): mount /orgs, /projects, /tasks, /billing, ...
		})
	})

	return r
}
