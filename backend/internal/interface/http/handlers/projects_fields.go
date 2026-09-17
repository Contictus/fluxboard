// Custom field endpoints (docs/08, FR-FIELDS). Definitions are managed by
// project LEADs; values are set by CONTRIBUTORs and read by VIEWERs (gated
// in projectuc).
package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/projectuc"
)

type customFieldResp struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Position int      `json:"position"`
}

func toCustomFieldResp(f *project.CustomField) customFieldResp {
	opts := f.Options
	if opts == nil {
		opts = []string{}
	}
	return customFieldResp{ID: f.ID, Name: f.Name, Type: f.Type, Options: opts, Position: f.Position}
}

type customValueResp struct {
	FieldID string   `json:"field_id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Text    *string  `json:"text,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Date    *string  `json:"date,omitempty"`
}

func toCustomValueResp(v *project.CustomValue) customValueResp {
	out := customValueResp{FieldID: v.FieldID, Name: v.Name, Type: v.Type, Text: v.Text, Number: v.Number}
	if v.Date != nil {
		s := v.Date.UTC().Format("2006-01-02")
		out.Date = &s
	}
	return out
}

type createFieldReq struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Position int      `json:"position"`
}

// ListFields returns definitions in position order (VIEWER).
func (h *ProjectHandlers) ListFields(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	fields, err := h.svc.ListFields(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]customFieldResp, 0, len(fields))
	for i := range fields {
		out = append(out, toCustomFieldResp(&fields[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"fields": out})
}

// CreateField stores a field definition (LEAD).
func (h *ProjectHandlers) CreateField(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req createFieldReq
	if !decodeJSON(w, r, &req) {
		return
	}
	f, err := h.svc.CreateField(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), projectuc.CreateFieldInput{
		Name: req.Name, Type: req.Type, Options: req.Options, Position: req.Position,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	// Re-read so created_at/position defaults are reflected.
	fields, err := h.svc.ListFields(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	for i := range fields {
		if fields[i].ID == f.ID {
			response.JSON(w, http.StatusCreated, toCustomFieldResp(&fields[i]))
			return
		}
	}
	response.JSON(w, http.StatusCreated, toCustomFieldResp(f))
}

type updateFieldReq struct {
	Name     string   `json:"name"`
	Options  []string `json:"options"`
	Position int      `json:"position"`
}

// UpdateField renames, re-options, or repositions a field (LEAD).
func (h *ProjectHandlers) UpdateField(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req updateFieldReq
	if !decodeJSON(w, r, &req) {
		return
	}
	f, err := h.svc.UpdateField(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "fieldId"), req.Name, req.Options, req.Position, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toCustomFieldResp(f))
}

// DeleteField removes a definition; values cascade (LEAD).
func (h *ProjectHandlers) DeleteField(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteField(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "fieldId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setFieldValueReq struct {
	Text   *string  `json:"text"`
	Number *float64 `json:"number"`
	Date   *string  `json:"date"`
}

// SetFieldValue validates against the field type and upserts (CONTRIBUTOR).
func (h *ProjectHandlers) SetFieldValue(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req setFieldValueReq
	if !decodeJSON(w, r, &req) {
		return
	}
	v := project.CustomValue{Text: req.Text, Number: req.Number}
	if req.Date != nil && *req.Date != "" {
		parsed, err := time.Parse("2006-01-02", *req.Date)
		if err != nil {
			response.Error(w, domain.ErrValidation)
			return
		}
		v.Date = &parsed
	}
	if err := h.svc.SetFieldValue(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "fieldId"), v, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TaskFieldValues returns a task's values in field order (VIEWER).
func (h *ProjectHandlers) TaskFieldValues(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	values, err := h.svc.TaskFieldValues(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]customValueResp, 0, len(values))
	for i := range values {
		out = append(out, toCustomValueResp(&values[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"values": out})
}

// ClearFieldValue removes one task value (CONTRIBUTOR).
func (h *ProjectHandlers) ClearFieldValue(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.ClearFieldValue(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "fieldId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
