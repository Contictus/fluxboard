package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
)

// errorBody is the single error envelope shape for the whole API
// (docs/08-API-SPEC.md error contract).
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// statusFor maps domain sentinel errors to HTTP status codes in exactly one
// place (docs/CLAUDE.md §Code Conventions). Handlers return domain errors;
// only this function knows about status codes.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrPlanLimit):
		return http.StatusPaymentRequired, "plan_limit"
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, "validation"
	default:
		return http.StatusInternalServerError, "internal"
	}
}

// WriteError serializes err as the standard envelope with the mapped status.
// Internal errors never leak their message to the client.
func WriteError(w http.ResponseWriter, err error) {
	status, code := statusFor(err)
	var body errorBody
	body.Error.Code = code
	if status == http.StatusInternalServerError {
		body.Error.Message = "internal server error"
	} else {
		body.Error.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
