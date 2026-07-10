// Package middleware implements the HTTP middleware chain. The install order in
// router.go is a contract (docs/03-ARCHITECTURE.md §2); do not reorder casually.
//
// Phase 0 ships the infrastructure middleware (RequestID, Logger, Recoverer,
// CORS) as real implementations. The security middleware (Auth, TenantResolver,
// RBAC, Entitlement, RateLimit) are pass-through stubs here and are filled in
// during Phases 1–4. Each stub is a named function so the chain in router.go
// already reads in its final shape.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyLogger
)

// RequestID assigns a UUIDv7 to every request, exposes it on the response as
// X-Request-ID, and stores it in the context for downstream logging.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuidv7.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the request id stored by RequestID, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID).(string)
	return id
}

// Logger emits one structured (slog JSON) line per request with the base fields
// from docs/10-INFRA-DEVOPS.md §5. user_id/org_id are attached by the auth and
// tenant middleware once those exist.
func Logger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := RequestIDFrom(r.Context())
			l := base.With("request_id", reqID)
			ctx := context.WithValue(r.Context(), ctxKeyLogger, l)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r.WithContext(ctx))

			l.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

// LoggerFrom returns the request-scoped logger, or slog.Default().
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// HTTPMetrics is the observation surface for the metrics middleware, implemented
// in cmd/api over a Prometheus histogram. Declared here as an interface so the
// middleware package stays off the prometheus import (mirrors RequestCounter).
type HTTPMetrics interface {
	// ObserveRequest records one served request. route is the chi route PATTERN
	// (e.g. /api/v1/orgs/{orgId}/projects) — bounded cardinality, never the raw path.
	ObserveRequest(method, route string, status int, seconds float64)
}

// Metrics records request duration by route pattern, method, and status. It runs
// in the infrastructure chain, so it reads the matched route pattern AFTER the
// downstream handler returns (chi fills the RouteContext during routing). An
// unmatched request (404) reports route="unmatched" to bound cardinality.
func Metrics(m HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			m.ObserveRequest(r.Method, route, sw.status, time.Since(start).Seconds())
		})
	}
}

// Recoverer turns a panic into a 500 and logs the stack. The stack never
// reaches the client.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				LoggerFrom(r.Context()).Error("panic recovered", "panic", rec)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"internal server error"}}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS allows the configured web origin(s) with credentials, for the browser SPA.
func CORS(allowedOrigins ...string) func(http.Handler) http.Handler {
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allow[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allow[origin]; ok {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-ID,Idempotency-Key,Last-Event-ID")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter captures the response status for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Flush implements http.Flusher, delegating to the wrapped writer so streaming
// handlers (SSE, docs/09 §1) can flush through the logging middleware. Without
// this, the wrapper masks the underlying Flusher and SSE cannot stream.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		s.wrote = true
		f.Flush()
	}
}
