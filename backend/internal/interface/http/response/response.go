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

// statusFor is the one place domain sentinels become HTTP status codes
// (docs/CLAUDE.md §Code Conventions). Codes match docs/08-API-SPEC.md §1.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, "validation_failed"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrPlanLimit):
		return http.StatusPaymentRequired, "plan_limit_exceeded"
	default:
		return http.StatusInternalServerError, "internal"
	}
}
