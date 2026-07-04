// Package httpx builds the API's HTTP surface: the chi router, the
// contract-ordered middleware chain, health probes, and the single
// domain-error-to-status map (errmap.go).
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
)

// Deps are the collaborators the router needs. Everything is an interface or a
// value so main.go owns construction (no global state — docs/CLAUDE.md).
type Deps struct {
	Logger      *slog.Logger
	WebOrigin   string
	Health      Health
	MetricsHTTP http.Handler // promhttp handler for /metrics
}

// NewRouter assembles the router. The infrastructure middleware wrap every
// route; the security middleware wrap only the versioned API group, in the
// exact order fixed by docs/03-ARCHITECTURE.md §2.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Infrastructure chain (1–4 of the contract), applied to all routes.
	r.Use(mw.RequestID)
	r.Use(mw.Logger(d.Logger))
	r.Use(mw.Recoverer)
	r.Use(mw.CORS(d.WebOrigin))

	// Operational endpoints live outside the auth chain.
	r.Get("/healthz", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)
	r.Handle("/metrics", d.MetricsHTTP)

	// Versioned API. Security chain (5–9) goes here; handlers are mounted by
	// later phases. Present now so the boundary is real from day one.
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(mw.Auth)           // 5
		api.Use(mw.TenantResolver) // 6
		api.Use(mw.RBAC)           // 7
		api.Use(mw.Entitlement)    // 8
		api.Use(mw.RateLimit)      // 9

		// TODO(phase1+): mount resource handlers here.
		api.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"pong": "v1"})
		})
	})

	return r
}
