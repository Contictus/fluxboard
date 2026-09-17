// This file is the tasks / subtasks / comments / labels surface (docs/01 §TASK).
// Project-scoped create/list live under /projects/{projectId}/tasks; task-by-id
// routes are flat under /tasks/{taskId}. Org labels are under /labels. All run
// under TenantGuard; project-role checks live in taskuc.
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
)

// TaskHandlers serves the task surface.
type TaskHandlers struct {
	svc    *taskuc.Service
	logger *slog.Logger
}

// NewTaskHandlers builds TaskHandlers.
func NewTaskHandlers(svc *taskuc.Service, logger *slog.Logger) *TaskHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &TaskHandlers{svc: svc, logger: logger}
}

// ---- DTOs -----------------------------------------------------------------

type taskResp struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	ColumnID    string     `json:"column_id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *string    `json:"assignee_id,omitempty"`
	Priority    string     `json:"priority"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Rank        string     `json:"rank"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func toTaskResp(t *project.Task) taskResp {
	return taskResp{
		ID: t.ID, ProjectID: t.ProjectID, ColumnID: t.ColumnID, Number: t.Number,
		Title: t.Title, Description: t.Description, AssigneeID: t.AssigneeID,
		Priority: string(t.Priority), StartDate: t.StartDate, DueDate: t.DueDate, Rank: t.Rank,
		CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

type subtaskResp struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
	Rank  string `json:"rank"`
}

type commentResp struct {
	ID        string     `json:"id"`
	AuthorID  string     `json:"author_id"`
	Body      string     `json:"body"`
	Edited    bool       `json:"edited"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func toCommentResp(c *project.Comment) commentResp {
	body := c.Body
	if c.Deleted() {
		body = "" // placeholder; the client renders "comment deleted"
	}
	return commentResp{
		ID: c.ID, AuthorID: c.AuthorID, Body: body, Edited: c.Edited,
		DeletedAt: c.DeletedAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

type labelResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func toLabelResp(l *project.Label) labelResp {
	return labelResp{ID: l.ID, Name: l.Name, Color: l.Color}
}

type activityResp struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Field     string    `json:"field"`
	OldValue  *string   `json:"old_value,omitempty"`
	NewValue  *string   `json:"new_value,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- Tasks ----------------------------------------------------------------

type createTaskReq struct {
	ColumnID    string     `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *string    `json:"assignee_id"`
	Priority    string     `json:"priority"`
	StartDate   *time.Time `json:"start_date"`
	DueDate     *time.Time `json:"due_date"`
}

// CreateTask creates a task in a project (CONTRIBUTOR+).
func (h *TaskHandlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req createTaskReq
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.CreateTask(r.Context(), tc.OrgID, tc.UserID, taskuc.CreateTaskInput{
		ProjectID: chi.URLParam(r, "projectId"), ColumnID: req.ColumnID, Title: req.Title,
		Description: req.Description, AssigneeID: req.AssigneeID,
		Priority: project.Priority(req.Priority), StartDate: req.StartDate, DueDate: req.DueDate,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toTaskResp(t))
}

// ListTasks lists a column's tasks (?column_id=).
func (h *TaskHandlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	columnID := r.URL.Query().Get("column_id")
	if columnID == "" {
		response.Error(w, domain.ErrValidation)
		return
	}
	ts, err := h.svc.ListTasksByColumn(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), columnID, tc.Role)
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

// GetTask returns one task.
func (h *TaskHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	t, err := h.svc.GetTask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toTaskResp(t))
}

type updateTaskReq struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *string    `json:"assignee_id"`
	Priority    string     `json:"priority"`
	StartDate   *time.Time `json:"start_date"`
	DueDate     *time.Time `json:"due_date"`
}

// UpdateTask edits a task's fields (CONTRIBUTOR+).
func (h *TaskHandlers) UpdateTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req updateTaskReq
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.UpdateTask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), taskuc.UpdateTaskInput{
		Title: req.Title, Description: req.Description, AssigneeID: req.AssigneeID,
		Priority: project.Priority(req.Priority), StartDate: req.StartDate, DueDate: req.DueDate,
	}, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toTaskResp(t))
}

// ListActivity returns a task's change history.
func (h *TaskHandlers) ListActivity(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	as, err := h.svc.ListActivity(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]activityResp, 0, len(as))
	for _, a := range as {
		out = append(out, activityResp{
			ID: a.ID, ActorID: a.ActorID, Field: a.Field,
			OldValue: a.OldValue, NewValue: a.NewValue, CreatedAt: a.CreatedAt,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"activity": out})
}

// ---- Subtasks -------------------------------------------------------------

type subtaskReq struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// AddSubtask appends a subtask (CONTRIBUTOR+).
func (h *TaskHandlers) AddSubtask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req subtaskReq
	if !decodeJSON(w, r, &req) {
		return
	}
	st, err := h.svc.AddSubtask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), req.Title, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, subtaskResp{ID: st.ID, Title: st.Title, Done: st.Done, Rank: st.Rank})
}

// ListSubtasks returns a task's subtasks.
func (h *TaskHandlers) ListSubtasks(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	sts, err := h.svc.ListSubtasks(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]subtaskResp, 0, len(sts))
	for _, st := range sts {
		out = append(out, subtaskResp{ID: st.ID, Title: st.Title, Done: st.Done, Rank: st.Rank})
	}
	response.JSON(w, http.StatusOK, map[string]any{"subtasks": out})
}

// UpdateSubtask edits a subtask (CONTRIBUTOR+).
func (h *TaskHandlers) UpdateSubtask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req subtaskReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.UpdateSubtask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "subtaskId"), req.Title, req.Done, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteSubtask removes a subtask (CONTRIBUTOR+).
func (h *TaskHandlers) DeleteSubtask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteSubtask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "subtaskId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Comments -------------------------------------------------------------

type commentReq struct {
	Body string `json:"body"`
}

// AddComment posts a comment (CONTRIBUTOR+).
func (h *TaskHandlers) AddComment(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req commentReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := h.svc.AddComment(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), req.Body, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toCommentResp(c))
}

// ListComments returns a task's comments.
func (h *TaskHandlers) ListComments(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	cs, err := h.svc.ListComments(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]commentResp, 0, len(cs))
	for i := range cs {
		out = append(out, toCommentResp(&cs[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"comments": out})
}

// EditComment edits a comment (author, within window).
func (h *TaskHandlers) EditComment(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req commentReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := h.svc.EditComment(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "commentId"), req.Body, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toCommentResp(c))
}

// DeleteComment soft-deletes a comment (author or LEAD).
func (h *TaskHandlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteComment(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "commentId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Labels ---------------------------------------------------------------

type labelReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateLabel creates an org label (MEMBER+; write:labels gate).
func (h *TaskHandlers) CreateLabel(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req labelReq
	if !decodeJSON(w, r, &req) {
		return
	}
	l, err := h.svc.CreateLabel(r.Context(), tc.OrgID, req.Name, req.Color)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toLabelResp(l))
}

// ListLabels returns the org's labels.
func (h *TaskHandlers) ListLabels(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	ls, err := h.svc.ListLabels(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]labelResp, 0, len(ls))
	for i := range ls {
		out = append(out, toLabelResp(&ls[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"labels": out})
}

// UpdateLabel edits a label (MEMBER+).
func (h *TaskHandlers) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req labelReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.UpdateLabel(r.Context(), tc.OrgID, chi.URLParam(r, "labelId"), req.Name, req.Color); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteLabel removes a label; it detaches everywhere (MEMBER+).
func (h *TaskHandlers) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DeleteLabel(r.Context(), tc.OrgID, chi.URLParam(r, "labelId")); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type attachLabelReq struct {
	LabelID string `json:"label_id"`
}

// AttachLabel links a label to a task (CONTRIBUTOR+).
func (h *TaskHandlers) AttachLabel(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req attachLabelReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.AttachLabel(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), req.LabelID, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DetachLabel unlinks a label from a task (CONTRIBUTOR+).
func (h *TaskHandlers) DetachLabel(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.DetachLabel(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), chi.URLParam(r, "labelId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListTaskLabels returns a task's labels.
func (h *TaskHandlers) ListTaskLabels(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	ls, err := h.svc.ListTaskLabels(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]labelResp, 0, len(ls))
	for i := range ls {
		out = append(out, toLabelResp(&ls[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"labels": out})
}
