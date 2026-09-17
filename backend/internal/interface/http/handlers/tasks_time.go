// Time tracking endpoints (docs/08, FR-TIME). Task-scoped; writes need
// CONTRIBUTOR on the task's project (gated in taskuc), reads need VIEWER.
package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
)

type timeEntryResp struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	UserID    string     `json:"user_id"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Note      string     `json:"note"`
	Seconds   int64      `json:"seconds"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListTimeEntries returns a task's entries newest-first (VIEWER).
func (h *TaskHandlers) ListTimeEntries(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	entries, err := h.svc.ListTimeEntries(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	now := time.Now().UTC()
	out := make([]timeEntryResp, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		out = append(out, timeEntryResp{
			ID: e.ID, TaskID: e.TaskID, UserID: e.UserID,
			StartedAt: e.StartedAt, EndedAt: e.EndedAt, Note: e.Note,
			Seconds: e.Seconds(now), CreatedAt: e.CreatedAt,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"entries": out})
}

// StartTimer stops the caller's running timers and starts one on the task.
func (h *TaskHandlers) StartTimer(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	e, err := h.svc.StartTimer(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	now := time.Now().UTC()
	response.JSON(w, http.StatusCreated, timeEntryResp{
		ID: e.ID, TaskID: e.TaskID, UserID: e.UserID,
		StartedAt: e.StartedAt, EndedAt: e.EndedAt, Note: e.Note,
		Seconds: e.Seconds(now), CreatedAt: e.CreatedAt,
	})
}

// StopTimer stops the caller's running entry.
func (h *TaskHandlers) StopTimer(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	e, err := h.svc.StopTimer(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "entryId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	now := time.Now().UTC()
	response.JSON(w, http.StatusOK, timeEntryResp{
		ID: e.ID, TaskID: e.TaskID, UserID: e.UserID,
		StartedAt: e.StartedAt, EndedAt: e.EndedAt, Note: e.Note,
		Seconds: e.Seconds(now), CreatedAt: e.CreatedAt,
	})
}

type logTimeReq struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Note      string    `json:"note"`
}

// LogTime records a finished manual entry.
func (h *TaskHandlers) LogTime(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req logTimeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	e, err := h.svc.LogTime(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), taskuc.LogTimeInput{
		StartedAt: req.StartedAt, EndedAt: req.EndedAt, Note: req.Note,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	now := time.Now().UTC()
	response.JSON(w, http.StatusCreated, timeEntryResp{
		ID: e.ID, TaskID: e.TaskID, UserID: e.UserID,
		StartedAt: e.StartedAt, EndedAt: e.EndedAt, Note: e.Note,
		Seconds: e.Seconds(now), CreatedAt: e.CreatedAt,
	})
}

// DeleteTimeEntry removes an entry (owner only).
func (h *TaskHandlers) DeleteTimeEntry(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteTimeEntry(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "entryId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
