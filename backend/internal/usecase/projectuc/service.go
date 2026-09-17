// Package projectuc holds the application services for projects and their boards
// (docs/01 §PROJ). It depends only on domain ports; ordering keys come from
// internal/pkg/rank and tenant isolation is enforced one layer down by RLS.
//
// Authorization is two-tier: the org-role Casbin gate (write:projects to create,
// read:org to view) runs in the HTTP middleware; the fine-grained project-role
// gate (LEAD/CONTRIBUTOR/VIEWER, with org ADMIN+ as implicit LEAD) lives here
// (ADR-013).
package projectuc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/rank"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// Deps are the collaborators the service needs.
type Deps struct {
	Projects   project.ProjectRepository
	Members    project.ProjectMemberRepository
	Boards     project.BoardRepository
	Columns    project.ColumnRepository
	Tasks      project.TaskRepository
	Sprints    project.SprintRepository       // sprints; nil ⇒ sprint endpoints disabled
	Fields     project.CustomFieldRepository  // custom fields; nil ⇒ field endpoints disabled
	Forms      project.FormRepository          // intake forms; nil ⇒ form endpoints disabled
	Labels     project.LabelRepository        // optional; nil ⇒ board cards carry no labels
	Subtasks   project.SubtaskRepository      // optional; nil ⇒ board cards carry no subtask progress
	Comments   project.CommentRepository      // optional; nil ⇒ board cards carry no comment counts
	Automation automation.Evaluator           // automation rules; nil ⇒ no evaluation
	Links      project.TaskLinkRepository  // dependency guard on moves; nil ⇒ no guard
	Events     notify.EventBus                // realtime publish; nil ⇒ no SSE events
	Logger     *slog.Logger
	Now        func() time.Time // injectable for tests; defaults to time.Now
}

// Service implements the project/board application logic.
type Service struct {
	projects   project.ProjectRepository
	members    project.ProjectMemberRepository
	boards     project.BoardRepository
	columns    project.ColumnRepository
	tasks      project.TaskRepository
	sprints    project.SprintRepository
	fields     project.CustomFieldRepository
	forms      project.FormRepository
	labels     project.LabelRepository
	subtasks   project.SubtaskRepository
	comments   project.CommentRepository
	automation automation.Evaluator
	links      project.TaskLinkRepository
	events     notify.EventBus
	logger     *slog.Logger
	now        func() time.Time
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		projects: d.Projects, members: d.Members, boards: d.Boards,
		columns: d.Columns, tasks: d.Tasks, sprints: d.Sprints, fields: d.Fields, forms: d.Forms, labels: d.Labels, subtasks: d.Subtasks,
		comments: d.Comments, automation: d.Automation, links: d.Links, events: d.Events, logger: logger, now: now,
	}
}

func newID() string { return uuidv7.New().String() }

// checkBlockers enforces the dependency guard: when columnID is the final
// column of boardID, every blocker of taskID must already sit in its own
// final column, else ErrConflict. Nil links repo ⇒ no guard.
func (s *Service) checkBlockers(ctx context.Context, orgID, boardID, columnID, taskID string) error {
	if s.links == nil {
		return nil
	}
	cols, err := s.columns.ListByBoard(ctx, orgID, boardID)
	if err != nil || len(cols) == 0 {
		return err
	}
	if cols[len(cols)-1].ID != columnID {
		return nil // only the final column is guarded
	}
	blockers, err := s.links.Blockers(ctx, orgID, taskID)
	if err != nil {
		return err
	}
	finalByProject := make(map[string]string)
	for _, b := range blockers {
		final, ok := finalByProject[b.ProjectID]
		if !ok {
			final = s.finalColumnID(ctx, orgID, b.ProjectID)
			finalByProject[b.ProjectID] = final
		}
		if b.ColumnID != final {
			return domain.ErrConflict
		}
	}
	return nil
}

// automate evaluates automation rules best-effort (nil ⇒ disabled). It never
// fails the write that triggered it.
func (s *Service) automate(ctx context.Context, e automation.Event) {
	if s.automation == nil {
		return
	}
	s.automation.Evaluate(ctx, e)
}

// publish emits a realtime event best-effort (a publish failure is logged, not
// returned — realtime is off the critical write path).
func (s *Service) publish(ctx context.Context, orgID, name, actorID string, data map[string]any) {
	if s.events == nil {
		return
	}
	if _, err := s.events.Publish(ctx, orgID, notify.NewEvent(name, actorID, data)); err != nil {
		s.logger.Warn("event publish failed", "event", name, "err", err)
	}
}

// ---- Inputs ---------------------------------------------------------------

// CreateProjectInput is the payload for CreateProject.
type CreateProjectInput struct {
	Key         string
	Name        string
	Description string
	Color       string
	Visibility  project.Visibility
}

// UpdateProjectInput is the payload for UpdateProject.
type UpdateProjectInput struct {
	Name        string
	Description string
	Color       string
	Visibility  project.Visibility
}

// ColumnView is a board column together with its ordered tasks. Labels maps
// task ID to that task's labels, Subtasks to checklist progress, and Comments
// to live comment counts (each empty when its dep is nil).
type ColumnView struct {
	Column   project.Column
	Tasks    []project.Task
	Labels   map[string][]project.Label
	Subtasks map[string]project.SubtaskCount
	Comments map[string]int
}

// BoardView is the kanban projection: the default board and its columns+tasks.
type BoardView struct {
	Board   project.Board
	Columns []ColumnView
}

// ---- Authorization --------------------------------------------------------

// effectiveRole resolves the caller's project role. Org ADMIN+ are implicit
// LEAD (FR-PROJ-003). A non-member sees an 'org'-visible project as VIEWER
// (unless GUEST); a non-member of a 'private' project gets ErrNotFound so the
// project stays opaque (FR-PROJ-002).
func (s *Service) effectiveRole(ctx context.Context, orgID string, p *project.Project, userID string, orgRole tenant.OrgRole) (project.ProjectRole, error) {
	if orgRole.AtLeast(tenant.RoleAdmin) {
		return project.RoleLead, nil
	}
	m, err := s.members.Get(ctx, orgID, p.ID, userID)
	if err == nil {
		return m.Role, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", err
	}
	if p.Visibility == project.VisibilityOrg && orgRole.AtLeast(tenant.RoleMember) {
		return project.RoleViewer, nil
	}
	return "", domain.ErrNotFound
}

// access loads a project and the caller's effective role, requiring at least
// min. Below min → ErrForbidden; not visible → ErrNotFound.
func (s *Service) access(ctx context.Context, orgID, projectID, userID string, orgRole tenant.OrgRole, min project.ProjectRole) (*project.Project, project.ProjectRole, error) {
	p, err := s.projects.Get(ctx, orgID, projectID)
	if err != nil {
		return nil, "", err
	}
	role, err := s.effectiveRole(ctx, orgID, p, userID, orgRole)
	if err != nil {
		return nil, "", err
	}
	if !role.AtLeast(min) {
		return nil, "", domain.ErrForbidden
	}
	return p, role, nil
}

// ---- Projects -------------------------------------------------------------

// CreateProject creates a project, its default board and the four seeded columns,
// and makes the creator a project LEAD (FR-PROJ-001/003/004). The org-role gate
// (MEMBER+ write:projects) is enforced by the caller's middleware.
func (s *Service) CreateProject(ctx context.Context, orgID, userID string, in CreateProjectInput) (*project.Project, error) {
	in.Key = strings.ToUpper(strings.TrimSpace(in.Key))
	in.Name = strings.TrimSpace(in.Name)
	if !project.ValidKey(in.Key) {
		return nil, domain.ErrValidation
	}
	if in.Name == "" {
		return nil, domain.ErrValidation
	}
	if in.Visibility == "" {
		in.Visibility = project.VisibilityOrg
	}
	if !in.Visibility.Valid() {
		return nil, domain.ErrValidation
	}

	// Plan project-limit (FR-PROJ-001) is enforced at the route via
	// EntitlementGuard.RequireProjects() (interface/http/router.go), so no
	// usecase-level check is needed here.

	p := &project.Project{
		ID: newID(), OrgID: orgID, Key: in.Key, Name: in.Name,
		Description: in.Description, Color: in.Color, Visibility: in.Visibility,
		CreatedBy: userID,
	}
	if err := s.projects.Create(ctx, orgID, p); err != nil {
		return nil, err // ErrConflict on duplicate key
	}
	if err := s.members.Add(ctx, orgID, p.ID, userID, project.RoleLead); err != nil {
		return nil, err
	}
	board := &project.Board{ID: newID(), ProjectID: p.ID, Name: "Board"}
	if err := s.boards.Create(ctx, orgID, board); err != nil {
		return nil, err
	}
	ranks := rank.Initial(len(project.DefaultColumns))
	for i, name := range project.DefaultColumns {
		col := &project.Column{ID: newID(), BoardID: board.ID, Name: name, Rank: ranks[i]}
		if err := s.columns.Create(ctx, orgID, col); err != nil {
			return nil, err
		}
	}
	return s.projects.Get(ctx, orgID, p.ID)
}

// ListProjects returns the projects visible to the caller (FR-PROJ-002).
func (s *Service) ListProjects(ctx context.Context, orgID, userID string, orgRole tenant.OrgRole, includeArchived bool) ([]project.Project, error) {
	seeAll := orgRole.AtLeast(tenant.RoleAdmin)
	return s.projects.List(ctx, orgID, userID, seeAll, includeArchived)
}

// GetProject returns a single project the caller may view.
func (s *Service) GetProject(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) (*project.Project, error) {
	p, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer)
	return p, err
}

// UpdateProject edits project settings (LEAD only, FR-PROJ-003). Archived
// projects are read-only (FR-PROJ-006).
func (s *Service) UpdateProject(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole, in UpdateProjectInput) (*project.Project, error) {
	p, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead)
	if err != nil {
		return nil, err
	}
	if p.Archived() {
		return nil, domain.ErrConflict
	}
	if strings.TrimSpace(in.Name) == "" || !in.Visibility.Valid() {
		return nil, domain.ErrValidation
	}
	p.Name = strings.TrimSpace(in.Name)
	p.Description = in.Description
	p.Color = in.Color
	p.Visibility = in.Visibility
	if err := s.projects.Update(ctx, orgID, p); err != nil {
		return nil, err
	}
	return s.projects.Get(ctx, orgID, projectID)
}

// SetArchived archives or unarchives a project (LEAD only, FR-PROJ-006).
func (s *Service) SetArchived(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole, archived bool) error {
	_, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead)
	if err != nil {
		return err
	}
	var at *time.Time
	if archived {
		now := s.now().UTC()
		at = &now
	}
	return s.projects.SetArchived(ctx, orgID, projectID, at)
}

// ---- Project members ------------------------------------------------------

// ListMembers returns a project's members (any viewer).
func (s *Service) ListMembers(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) ([]project.ProjectMember, error) {
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.members.List(ctx, orgID, projectID)
}

// AddMember adds or updates a project member's role (LEAD only).
func (s *Service) AddMember(ctx context.Context, orgID, userID, projectID, targetUserID string, role project.ProjectRole, orgRole tenant.OrgRole) error {
	if !role.Valid() {
		return domain.ErrValidation
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	return s.members.Add(ctx, orgID, projectID, targetUserID, role)
}

// RemoveMember removes a project member (LEAD only).
func (s *Service) RemoveMember(ctx context.Context, orgID, userID, projectID, targetUserID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	return s.members.Remove(ctx, orgID, projectID, targetUserID)
}

// ---- Board + columns ------------------------------------------------------

// GetBoard returns the kanban projection for a project (FR-PROJ-004/005).
func (s *Service) GetBoard(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) (*BoardView, error) {
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	board, err := s.boards.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	cols, err := s.columns.ListByBoard(ctx, orgID, board.ID)
	if err != nil {
		return nil, err
	}
	view := &BoardView{Board: *board}
	tasks, err := s.tasks.ListByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	tasksByColumn := make(map[string][]project.Task, len(cols))
	for _, c := range cols {
		tasksByColumn[c.ID] = make([]project.Task, 0)
	}
	for _, t := range tasks {
		tasksByColumn[t.ColumnID] = append(tasksByColumn[t.ColumnID], t)
	}
	labelsByTask := make(map[string][]project.Label)
	if s.labels != nil {
		if lb, err := s.labels.ListForProject(ctx, orgID, projectID); err == nil {
			labelsByTask = lb
		} else {
			s.logger.Warn("board labels enrichment failed", "err", err, "project", projectID)
		}
	}
	subtasksByTask := make(map[string]project.SubtaskCount)
	if s.subtasks != nil {
		if sc, err := s.subtasks.CountForProject(ctx, orgID, projectID); err == nil {
			subtasksByTask = sc
		} else {
			s.logger.Warn("board subtask enrichment failed", "err", err, "project", projectID)
		}
	}
	commentsByTask := make(map[string]int)
	if s.comments != nil {
		if cc, err := s.comments.CountForProject(ctx, orgID, projectID); err == nil {
			commentsByTask = cc
		} else {
			s.logger.Warn("board comment enrichment failed", "err", err, "project", projectID)
		}
	}
	for _, c := range cols {
		view.Columns = append(view.Columns, ColumnView{
			Column: c, Tasks: tasksByColumn[c.ID],
			Labels: labelsByTask, Subtasks: subtasksByTask, Comments: commentsByTask,
		})
	}
	return view, nil
}

// AddColumn appends a column to a project's board (LEAD only, FR-PROJ-004).
func (s *Service) AddColumn(ctx context.Context, orgID, userID, projectID, name string, wipLimit *int, orgRole tenant.OrgRole) (*project.Column, error) {
	if strings.TrimSpace(name) == "" {
		return nil, domain.ErrValidation
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	board, err := s.boards.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	cols, err := s.columns.ListByBoard(ctx, orgID, board.ID)
	if err != nil {
		return nil, err
	}
	last := ""
	if n := len(cols); n > 0 {
		last = cols[n-1].Rank
	}
	col := &project.Column{ID: newID(), BoardID: board.ID, Name: strings.TrimSpace(name), Rank: rank.Append(last), WIPLimit: wipLimit}
	if err := s.columns.Create(ctx, orgID, col); err != nil {
		return nil, err
	}
	return s.columns.Get(ctx, orgID, col.ID)
}

// RenameColumn renames a column and sets its WIP limit (LEAD only).
func (s *Service) RenameColumn(ctx context.Context, orgID, userID, projectID, columnID, name string, wipLimit *int, orgRole tenant.OrgRole) error {
	if strings.TrimSpace(name) == "" {
		return domain.ErrValidation
	}
	if err := s.requireColumn(ctx, orgID, userID, projectID, columnID, orgRole); err != nil {
		return err
	}
	return s.columns.Update(ctx, orgID, columnID, strings.TrimSpace(name), wipLimit)
}

// ReorderColumn moves a column to the given rank (LEAD only). The client computes
// the rank between the target neighbours (same scheme as task moves).
func (s *Service) ReorderColumn(ctx context.Context, orgID, userID, projectID, columnID, newRank string, orgRole tenant.OrgRole) error {
	if newRank == "" {
		return domain.ErrValidation
	}
	if err := s.requireColumn(ctx, orgID, userID, projectID, columnID, orgRole); err != nil {
		return err
	}
	return s.columns.SetRank(ctx, orgID, columnID, newRank)
}

// DeleteColumn removes a column, first migrating its tasks to targetColumnID
// (LEAD only, FR-PROJ-004). A non-empty column requires a migration target.
func (s *Service) DeleteColumn(ctx context.Context, orgID, userID, projectID, columnID, targetColumnID string, orgRole tenant.OrgRole) error {
	if err := s.requireColumn(ctx, orgID, userID, projectID, columnID, orgRole); err != nil {
		return err
	}
	tasks, err := s.tasks.ListByColumn(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		if targetColumnID == "" || targetColumnID == columnID {
			return domain.ErrValidation // must migrate tasks to another column
		}
		if err := s.requireColumn(ctx, orgID, userID, projectID, targetColumnID, orgRole); err != nil {
			return err
		}
		existing, err := s.tasks.ListByColumn(ctx, orgID, targetColumnID)
		if err != nil {
			return err
		}
		last := ""
		if n := len(existing); n > 0 {
			last = existing[n-1].Rank
		}
		for _, t := range tasks {
			last = rank.Append(last)
			if err := s.tasks.Move(ctx, orgID, t.ID, targetColumnID, last); err != nil {
				return err
			}
		}
	}
	return s.columns.Delete(ctx, orgID, columnID)
}

// MoveTask relocates a task to (columnID, rank) on the board (CONTRIBUTOR+,
// FR-PROJ-005). A rank collision (concurrent move) surfaces as ErrConflict → 409.
func (s *Service) MoveTask(ctx context.Context, orgID, userID, taskID, columnID, newRank string, orgRole tenant.OrgRole) error {
	if newRank == "" {
		return domain.ErrValidation
	}
	t, err := s.tasks.Get(ctx, orgID, taskID)
	if err != nil {
		return err
	}
	if _, _, err := s.access(ctx, orgID, t.ProjectID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	// The target column must belong to this project's board.
	col, err := s.columns.Get(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	board, err := s.boards.GetByProject(ctx, orgID, t.ProjectID)
	if err != nil {
		return err
	}
	if col.BoardID != board.ID {
		return domain.ErrValidation
	}
	// Dependency guard (FR-LINKS): moving into the final column fails while
	// open blockers exist. Open = blocker not in its own final column.
	if err := s.checkBlockers(ctx, orgID, board.ID, columnID, taskID); err != nil {
		return err
	}
	fromColumn := t.ColumnID
	if err := s.tasks.Move(ctx, orgID, taskID, columnID, newRank); err != nil {
		return err
	}
	s.publish(ctx, orgID, notify.EventTaskMoved, userID, map[string]any{
		"task_id": taskID, "project_id": t.ProjectID,
		"from_column": fromColumn, "to_column": columnID, "rank": newRank,
	})
	s.automate(ctx, automation.Event{
		OrgID: orgID, Trigger: automation.TriggerTaskMoved,
		TaskID: taskID, ActorID: userID, ToColumnID: columnID,
	})
	return nil
}

// requireColumn checks LEAD access on the project and that the column belongs to
// the project's board.
func (s *Service) requireColumn(ctx context.Context, orgID, userID, projectID, columnID string, orgRole tenant.OrgRole) error {
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	col, err := s.columns.Get(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	board, err := s.boards.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return err
	}
	if col.BoardID != board.ID {
		return domain.ErrNotFound
	}
	return nil
}
