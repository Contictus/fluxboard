// Dependency endpoints (docs/08, FR-LINKS). Directed "blocked by" edges;
// writes need CONTRIBUTOR, reads need VIEWER (gated in taskuc).
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

type taskLinkResp struct {
	TaskID   string `json:"task_id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	ColumnID string `json:"column_id"`
}

// ListTaskLinks returns both directions: blockers and blocked.
func (h *TaskHandlers) ListTaskLinks(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	links, err := h.svc.ListLinks(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	blockers := make([]taskLinkResp, 0, len(links.Blockers))
	for i := range links.Blockers {
		b := &links.Blockers[i]
		blockers = append(blockers, taskLinkResp{TaskID: b.LinkedTaskID, Number: b.Number, Title: b.Title, ColumnID: b.ColumnID})
	}
	blocked := make([]taskLinkResp, 0, len(links.Blocked))
	for i := range links.Blocked {
		b := &links.Blocked[i]
		blocked = append(blocked, taskLinkResp{TaskID: b.TaskID, Number: b.Number, Title: b.Title, ColumnID: b.ColumnID})
	}
	response.JSON(w, http.StatusOK, map[string]any{"blockers": blockers, "blocked": blocked})
}

type addLinkReq struct {
	LinkedTaskID string `json:"linked_task_id"`
}

// AddTaskLink records taskID blocked-by linkedID (CONTRIBUTOR).
func (h *TaskHandlers) AddTaskLink(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req addLinkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.AddLink(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), req.LinkedTaskID, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveTaskLink deletes an edge (CONTRIBUTOR).
func (h *TaskHandlers) RemoveTaskLink(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.RemoveLink(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "linkedId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
