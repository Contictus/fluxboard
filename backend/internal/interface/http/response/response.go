// Package response centralizes the JSON success/error envelope and the single
// domain-error-to-HTTP-status mapping (docs/08-API-SPEC.md §1). Both httpx and
// the handlers depend on it; it depends only on the domain, so there is no
// import cycle. It reads the request id from the already-set X-Request-ID
// response header rather than importing the middleware package.
package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
)

// Envelope is the error envelope shape shared by the whole API.
type Envelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the error payload.
type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// JSON writes v as a JSON body with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error maps err to the standard envelope + status. Internal errors never leak
// their message. 401s carry a WWW-Authenticate: Bearer challenge.
func Error(w http.ResponseWriter, err error) {
	status, code := statusFor(err)

	msg := err.Error()
	if status == http.StatusInternalServerError {
		msg = "internal server error"
	}

	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	JSON(w, status, Envelope{Error: ErrorBody{
		Code:      code,
		Message:   msg,
		RequestID: w.Header().Get("X-Request-ID"),
	}})
}

// PlanLimit writes the 402 plan_limit_exceeded envelope with a machine-readable
// body ("X of Y used") so the client can render the upgrade prompt (FR-BILL-009,
// docs/06-BILLING.md §7). limit is the ceiling key (e.g. "max_projects"). This
// is a richer variant of the ErrPlanLimit sentinel path, which carries no counts.
func PlanLimit(w http.ResponseWriter, limit string, current, max int64) {
	JSON(w, http.StatusPaymentRequired, Envelope{Error: ErrorBody{
		Code:      "plan_limit_exceeded",
		Message:   "plan limit exceeded",
		Details:   map[string]any{"limit": limit, "current": current, "max": max},
		RequestID: w.Header().Get("X-Request-ID"),
	}})
}

// RateLimited writes the 429 rate_limited envelope with the plan's per-minute
// ceiling and a Retry-After hint (FR-BILL-009, api_rate_per_min).
func RateLimited(w http.ResponseWriter, perMin int) {
	w.Header().Set("Retry-After", "60")
	JSON(w, http.StatusTooManyRequests, Envelope{Error: ErrorBody{
		Code:      "rate_limited",
		Message:   "API rate limit exceeded",
		Details:   map[string]any{"limit": "api_rate_per_min", "max": perMin},
		RequestID: w.Header().Get("X-Request-ID"),
	}})
}

// statusFor is the one place domain sentinels become HTTP status codes
// (docs/CLAUDE.md §Code Conventions). Codes match docs/08-API-SPEC.md §1.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, "validation_failed"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrEmailUnverified):
		return http.StatusForbidden, "email_unverified"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrPlanLimit):
		return http.StatusPaymentRequired, "plan_limit_exceeded"
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited"
	default:
		return http.StatusInternalServerError, "internal"
	}
}
