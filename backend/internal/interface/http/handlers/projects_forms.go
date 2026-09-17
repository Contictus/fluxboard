// Intake-form endpoints (ADR-023). Project-scoped management needs LEAD to
// write and VIEWER to list (gated in projectuc); token hashes never leave the
// server — the raw token is returned once at create/rotate. The public
// read/submit pair lives on PublicFormHandlers (no tenant context — the token
// is the credential) and is mounted outside auth in the router.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/projectuc"
)

type formResp struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	TargetColumnID string `json:"target_column_id"`
	IsActive       bool   `json:"is_active"`
	CreatedAt      string `json:"created_at"`
}

func toFormResp(f *project.ProjectForm) formResp {
	return formResp{
		ID: f.ID, Name: f.Name, Description: f.Description,
		TargetColumnID: f.TargetColumnID, IsActive: f.IsActive,
		CreatedAt: f.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

type createFormReq struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	TargetColumnID string `json:"target_column_id"`
}

type updateFormReq struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	TargetColumnID string `json:"target_column_id"`
	IsActive       bool   `json:"is_active"`
}

// ListForms returns a project's intake forms (VIEWER).
func (h *ProjectHandlers) ListForms(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	forms, err := h.svc.ListForms(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]formResp, 0, len(forms))
	for i := range forms {
		out = append(out, toFormResp(&forms[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"forms": out})
}

// CreateForm stores a form and reveals the raw token once (LEAD).
func (h *ProjectHandlers) CreateForm(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req createFormReq
	if !decodeJSON(w, r, &req) {
		return
	}
	f, raw, err := h.svc.CreateForm(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), projectuc.CreateFormInput{
		Name: req.Name, Description: req.Description, TargetColumnID: req.TargetColumnID,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := toFormResp(f)
	response.JSON(w, http.StatusCreated, map[string]any{"form": out, "token": raw})
}

// UpdateForm rewrites a form (LEAD).
func (h *ProjectHandlers) UpdateForm(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req updateFormReq
	if !decodeJSON(w, r, &req) {
		return
	}
	f, err := h.svc.UpdateForm(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "formId"), projectuc.UpdateFormInput{
		Name: req.Name, Description: req.Description,
		TargetColumnID: req.TargetColumnID, IsActive: req.IsActive,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toFormResp(f))
}

// RotateFormToken swaps a form's token, killing the old URL (LEAD).
func (h *ProjectHandlers) RotateFormToken(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	raw, err := h.svc.RotateFormToken(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "formId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"token": raw})
}

// DeleteForm removes a form; submitted tasks are untouched (LEAD).
func (h *ProjectHandlers) DeleteForm(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteForm(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "formId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Public surface ----------------------------------------------------------

// PublicFormHandlers serves the unauthenticated form page + submit. Mounted
// outside auth; the token in the path is the only credential (ADR-023).
type PublicFormHandlers struct {
	svc    *projectuc.Service
	logger *slog.Logger
}

// NewPublicFormHandlers builds PublicFormHandlers.
func NewPublicFormHandlers(svc *projectuc.Service, logger *slog.Logger) *PublicFormHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &PublicFormHandlers{svc: svc, logger: logger}
}

// GetPublicForm renders the anonymous form page data. Unknown/disabled forms
// read as 404 (no oracle).
func (h *PublicFormHandlers) GetPublicForm(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.GetPublicForm(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"name": f.Name, "description": f.Description})
}

type submitFormReq struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Priority       string `json:"priority"`
	SubmitterName  string `json:"submitter_name"`
	SubmitterEmail string `json:"submitter_email"`
}

// SubmitPublicForm files an anonymous task. Per-IP throttled at the route.
func (h *PublicFormHandlers) SubmitPublicForm(w http.ResponseWriter, r *http.Request) {
	var req submitFormReq
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.SubmitForm(r.Context(), chi.URLParam(r, "token"), projectuc.SubmitFormInput{
		Title: req.Title, Description: req.Description, Priority: req.Priority,
		SubmitterName: req.SubmitterName, SubmitterEmail: req.SubmitterEmail,
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, map[string]any{"id": t.ID, "number": t.Number})
}
