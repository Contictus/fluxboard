// Phase 3b task surface: Trash (soft-delete/restore), full-text search, bulk
// board actions and MinIO-backed attachments (docs/01 §TASK FR-TASK-006..009).
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// ---- Trash (FR-TASK-009) --------------------------------------------------

// TrashTask soft-deletes a task (CONTRIBUTOR+).
func (h *TaskHandlers) TrashTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.TrashTask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreTask restores a trashed task (CONTRIBUTOR+).
func (h *TaskHandlers) RestoreTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	t, err := h.svc.RestoreTask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toTaskResp(t))
}

// ListTrash returns a project's trashed tasks (VIEWER+).
func (h *TaskHandlers) ListTrash(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	ts, err := h.svc.ListTrash(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]taskResp, 0, len(ts))
	for i := range ts {
		out = append(out, toTaskResp(&ts[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"tasks": out})
}

// ---- Search (FR-TASK-007) -------------------------------------------------

// Search runs a paginated full-text task search (?q=&project_id=&assignee_id=
// &column_id=&priority=&label_id=&limit=&offset=).
func (h *TaskHandlers) Search(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	res, err := h.svc.Search(r.Context(), tc.OrgID, tc.UserID, project.SearchFilter{
		Query:      q.Get("q"),
		ProjectID:  q.Get("project_id"),
		AssigneeID: q.Get("assignee_id"),
		ColumnID:   q.Get("column_id"),
		Priority:   q.Get("priority"),
		LabelID:    q.Get("label_id"),
		Limit:      limit,
		Offset:     offset,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]taskResp, 0, len(res.Tasks))
	for i := range res.Tasks {
		out = append(out, toTaskResp(&res.Tasks[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"tasks": out, "total": res.Total})
}

// ---- Bulk actions (FR-TASK-008) -------------------------------------------

type bulkReq struct {
	Action     string   `json:"action"` // assign | move | label
	TaskIDs    []string `json:"task_ids"`
	AssigneeID *string  `json:"assignee_id"`
	ColumnID   string   `json:"column_id"`
	LabelID    string   `json:"label_id"`
}

// BulkAction applies one action to a set of tasks in a single transaction.
func (h *TaskHandlers) BulkAction(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req bulkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	var err error
	switch req.Action {
	case "assign":
		err = h.svc.BulkAssign(r.Context(), tc.OrgID, tc.UserID, req.TaskIDs, req.AssigneeID, tc.Role)
	case "move":
		if req.ColumnID == "" {
			err = domain.ErrValidation
			break
		}
		err = h.svc.BulkMove(r.Context(), tc.OrgID, tc.UserID, req.TaskIDs, req.ColumnID, tc.Role)
	case "label":
		if req.LabelID == "" {
			err = domain.ErrValidation
			break
		}
		err = h.svc.BulkAddLabel(r.Context(), tc.OrgID, tc.UserID, req.TaskIDs, req.LabelID, tc.Role)
	default:
		err = domain.ErrValidation
	}
	if err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Attachments (FR-TASK-006) --------------------------------------------

type attachmentResp struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id"`
	UploaderID  string     `json:"uploader_id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

func toAttachmentResp(a *project.Attachment) attachmentResp {
	return attachmentResp{
		ID: a.ID, TaskID: a.TaskID, UploaderID: a.UploaderID, Filename: a.Filename,
		ContentType: a.ContentType, SizeBytes: a.SizeBytes, Status: string(a.Status),
		CreatedAt: a.CreatedAt, ConfirmedAt: a.ConfirmedAt,
	}
}

type requestUploadReq struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// RequestUpload validates an upload and returns a presigned PUT URL (CONTRIBUTOR+).
func (h *TaskHandlers) RequestUpload(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req requestUploadReq
	if !decodeJSON(w, r, &req) {
		return
	}
	a, url, err := h.svc.RequestUpload(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"),
		req.Filename, req.ContentType, req.Size, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, map[string]any{
		"attachment": toAttachmentResp(a),
		"upload_url": url,
	})
}

// ConfirmUpload verifies the object exists and commits the attachment (CONTRIBUTOR+).
func (h *TaskHandlers) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	a, err := h.svc.ConfirmUpload(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "attachmentId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toAttachmentResp(a))
}

// ListAttachments returns a task's committed attachments (VIEWER+).
func (h *TaskHandlers) ListAttachments(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	as, err := h.svc.ListAttachments(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]attachmentResp, 0, len(as))
	for i := range as {
		out = append(out, toAttachmentResp(&as[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"attachments": out})
}

// DownloadAttachment returns a short-lived presigned GET URL (VIEWER+).
func (h *TaskHandlers) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	url, err := h.svc.DownloadURL(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "attachmentId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"download_url": url})
}

// DeleteAttachment removes an attachment (CONTRIBUTOR+).
func (h *TaskHandlers) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteAttachment(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "attachmentId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
