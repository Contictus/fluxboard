// Package domain holds the enterprise sentinel errors shared by every domain
// subpackage. The HTTP layer maps these to status codes in exactly one place
// (internal/interface/http/errmap.go). Domain packages import stdlib only.
package domain

import "errors"

var (
	// ErrNotFound: the requested entity does not exist (or is not visible to
	// the caller's tenant). Maps to 404.
	ErrNotFound = errors.New("not found")
	// ErrForbidden: authenticated but not authorized for this action. Maps to 403.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict: state precondition failed (e.g. duplicate, rank collision).
	// Maps to 409.
	ErrConflict = errors.New("conflict")
	// ErrPlanLimit: entitlement/plan quota exceeded. Maps to 402/403 per route.
	ErrPlanLimit = errors.New("plan limit exceeded")
	// ErrValidation: input failed validation. Maps to 400/422.
	ErrValidation = errors.New("validation failed")
)
