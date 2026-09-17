// This file is the projects / boards / columns surface (docs/01 §PROJ). All
// routes run under TenantGuard (resolve + Casbin org-role gate); the fine-grained
// project-role gate is enforced in projectuc. Handlers here trust the resolved
// TenantContext.
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
	"github.com/mesutokul/fluxboard/backend/internal/usecase/projectuc"
)

// ProjectHandlers serves the project/board surface.
type ProjectHandlers struct {
	svc    *projectuc.Service
	logger *slog.Logger
}

// NewProjectHandlers builds ProjectHandlers.
func NewProjectHandlers(svc *projectuc.Service, logger *slog.Logger) *ProjectHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProjectHandlers{svc: svc, logger: logger}
}

// ---- DTOs -----------------------------------------------------------------

type projectResp struct {
	ID          string     `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Color       string     `json:"color"`
	Visibility  string     `json:"visibility"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func toProjectResp(p *project.Project) projectResp {
	return projectResp{
		ID: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Color: p.Color,
		Visibility: string(p.Visibility), ArchivedAt: p.ArchivedAt, CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type columnResp struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rank     string `json:"rank"`
	WIPLimit *int   `json:"wip_limit,omitempty"`
}

func toColumnResp(c *project.Column) columnResp {
	return columnResp{ID: c.ID, Name: c.Name, Rank: c.Rank, WIPLimit: c.WIPLimit}
}

type taskCardResp struct {
	ID       string      `json:"id"`
	Number   int         `json:"number"`
	Title    string      `json:"title"`
	Priority string      `json:"priority"`
	Assignee *string     `json:"assignee_id,omitempty"`
	DueDate  *time.Time  `json:"due_date,omitempty"`
	Labels   []labelResp `json:"labels"`
	Rank     string      `json:"rank"`
}

type boardColumnResp struct {
	columnResp
	Tasks []taskCardResp `json:"tasks"`
}

type boardResp struct {
	BoardID string            `json:"board_id"`
	Columns []boardColumnResp `json:"columns"`
}

type memberResp2 struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- Projects -------------------------------------------------------------

type createProjectReq struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Visibility  string `json:"visibility"`
}

// CreateProject creates a project (MEMBER+; write:projects gate).
func (h *ProjectHandlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req createProjectReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.CreateProject(r.Context(), tc.OrgID, tc.UserID, projectuc.CreateProjectInput{
		Key: req.Key, Name: req.Name, Description: req.Description,
		Color: req.Color, Visibility: project.Visibility(req.Visibility),
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toProjectResp(p))
}

// ListProjects lists the caller's visible projects (?archived=true to include).
func (h *ProjectHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	includeArchived := r.URL.Query().Get("archived") == "true"
	ps, err := h.svc.ListProjects(r.Context(), tc.OrgID, tc.UserID, tc.Role, includeArchived)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]projectResp, 0, len(ps))
	for i := range ps {
		out = append(out, toProjectResp(&ps[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"projects": out})
}

// GetProject returns one project.
func (h *ProjectHandlers) GetProject(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	p, err := h.svc.GetProject(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toProjectResp(p))
}

type updateProjectReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Visibility  string `json:"visibility"`
}

// UpdateProject edits project settings (LEAD).
func (h *ProjectHandlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req updateProjectReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateProject(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role, projectuc.UpdateProjectInput{
		Name: req.Name, Description: req.Description, Color: req.Color, Visibility: project.Visibility(req.Visibility),
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toProjectResp(p))
}

// ArchiveProject archives a project (LEAD, FR-PROJ-006).
func (h *ProjectHandlers) ArchiveProject(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, true)
}

// UnarchiveProject restores an archived project (LEAD).
func (h *ProjectHandlers) UnarchiveProject(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, false)
}

func (h *ProjectHandlers) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.SetArchived(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role, archived); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Project members ------------------------------------------------------

// ListMembers lists a project's members.
func (h *ProjectHandlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	ms, err := h.svc.ListMembers(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]memberResp2, 0, len(ms))
	for _, m := range ms {
		out = append(out, memberResp2{UserID: m.UserID, Role: string(m.Role), CreatedAt: m.CreatedAt})
	}
	response.JSON(w, http.StatusOK, map[string]any{"members": out})
}

type addMemberReq struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// AddMember adds or updates a project member (LEAD).
func (h *ProjectHandlers) AddMember(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req addMemberReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.AddMember(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), req.UserID, project.ProjectRole(req.Role), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember removes a project member (LEAD).
func (h *ProjectHandlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "userId"), tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Board + columns ------------------------------------------------------

// GetBoard returns the kanban projection.
func (h *ProjectHandlers) GetBoard(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	view, err := h.svc.GetBoard(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	resp := boardResp{BoardID: view.Board.ID, Columns: []boardColumnResp{}}
	for _, cv := range view.Columns {
		bc := boardColumnResp{columnResp: toColumnResp(&cv.Column), Tasks: []taskCardResp{}}
		for i := range cv.Tasks {
			t := cv.Tasks[i]
			card := taskCardResp{
				ID: t.ID, Number: t.Number, Title: t.Title,
				Priority: string(t.Priority), Assignee: t.AssigneeID, DueDate: t.DueDate, Rank: t.Rank,
				Labels: []labelResp{},
			}
			for _, l := range cv.Labels[t.ID] {
				ll := l
				card.Labels = append(card.Labels, toLabelResp(&ll))
			}
			bc.Tasks = append(bc.Tasks, card)
		}
		resp.Columns = append(resp.Columns, bc)
	}
	response.JSON(w, http.StatusOK, resp)
}

type columnReq struct {
	Name     string `json:"name"`
	WIPLimit *int   `json:"wip_limit"`
}

// AddColumn appends a column (LEAD).
func (h *ProjectHandlers) AddColumn(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req columnReq
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := h.svc.AddColumn(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), req.Name, req.WIPLimit, tc.Role)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toColumnResp(c))
}

// RenameColumn renames a column / sets its WIP limit (LEAD).
func (h *ProjectHandlers) RenameColumn(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req columnReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.RenameColumn(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "columnId"), req.Name, req.WIPLimit, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reorderReq struct {
	Rank string `json:"rank"`
}

// ReorderColumn moves a column (LEAD).
func (h *ProjectHandlers) ReorderColumn(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req reorderReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ReorderColumn(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "columnId"), req.Rank, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteColumn removes a column, migrating tasks to ?target= (LEAD).
func (h *ProjectHandlers) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	target := r.URL.Query().Get("target")
	if err := h.svc.DeleteColumn(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"), chi.URLParam(r, "columnId"), target, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type positionReq struct {
	ColumnID string `json:"column_id"`
	Rank     string `json:"rank"`
}

// MoveTask relocates a task on the board (CONTRIBUTOR+, FR-PROJ-005). A rank
// collision returns 409.
func (h *ProjectHandlers) MoveTask(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req positionReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.MoveTask(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "taskId"), req.ColumnID, req.Rank, tc.Role); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
