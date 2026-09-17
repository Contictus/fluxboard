package projectuc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// ---- Sprints (FR-SPRINT) -----------------------------------------------------
// Sprints are managed by project LEADs. Only one sprint per project may be
// active; starting a sprint while another is active fails so scope stays
// explicit. Completing snapshots velocity and returns unfinished tasks to the
// backlog, mirroring the standard Scrum flow.

// CreateSprintInput carries the sprint planner form.
type CreateSprintInput struct {
	Name      string
	Goal      string
	StartedAt *time.Time
	EndedAt   *time.Time
}

// sprintsEnabled reports whether the sprint repository is wired.
func (s *Service) sprintsEnabled() bool { return s.sprints != nil }

// CreateSprint stores a planned sprint (LEAD).
func (s *Service) CreateSprint(ctx context.Context, orgID, userID, projectID string, in CreateSprintInput, orgRole tenant.OrgRole) (*project.Sprint, error) {
	if !s.sprintsEnabled() {
		return nil, domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, domain.ErrValidation
	}
	if in.StartedAt != nil && in.EndedAt != nil && !in.EndedAt.After(*in.StartedAt) {
		return nil, domain.ErrValidation
	}
	sp := &project.Sprint{
		ID: newID(), OrgID: orgID, ProjectID: projectID, Name: name,
		Goal: strings.TrimSpace(in.Goal), Status: project.SprintPlanned,
		StartedAt: in.StartedAt, EndedAt: in.EndedAt, CreatedBy: userID,
	}
	if err := s.sprints.Create(ctx, orgID, sp); err != nil {
		return nil, err
	}
	return s.sprints.Get(ctx, orgID, sp.ID)
}

// ListSprints returns a project's sprints oldest-first (VIEWER).
func (s *Service) ListSprints(ctx context.Context, orgID, userID, projectID string, orgRole tenant.OrgRole) ([]project.Sprint, error) {
	if !s.sprintsEnabled() {
		return nil, domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.sprints.ListByProject(ctx, orgID, projectID)
}

// StartSprint activates a planned sprint (LEAD). Fails when another sprint is
// already active in the project.
func (s *Service) StartSprint(ctx context.Context, orgID, userID, projectID, sprintID string, orgRole tenant.OrgRole) (*project.Sprint, error) {
	if !s.sprintsEnabled() {
		return nil, domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	sp, err := s.sprintInProject(ctx, orgID, projectID, sprintID)
	if err != nil {
		return nil, err
	}
	if sp.Status != project.SprintPlanned {
		return nil, domain.ErrConflict
	}
	if _, err := s.sprints.GetActive(ctx, orgID, projectID); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	now := s.now().UTC()
	sp.Status = project.SprintActive
	if sp.StartedAt == nil {
		sp.StartedAt = &now
	}
	if err := s.sprints.Update(ctx, orgID, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

// CompleteSprint closes the active sprint (LEAD): snapshots velocity (total
// scope vs tasks in the final column) and returns unfinished tasks to the
// backlog.
func (s *Service) CompleteSprint(ctx context.Context, orgID, userID, projectID, sprintID string, orgRole tenant.OrgRole) (*project.Sprint, error) {
	if !s.sprintsEnabled() {
		return nil, domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return nil, err
	}
	sp, err := s.sprintInProject(ctx, orgID, projectID, sprintID)
	if err != nil {
		return nil, err
	}
	if sp.Status != project.SprintActive {
		return nil, domain.ErrConflict
	}
	tasks, err := s.tasks.ListByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	doneColumn := s.finalColumnID(ctx, orgID, projectID)
	total, done := 0, 0
	for i := range tasks {
		t := &tasks[i]
		if t.SprintID == nil || *t.SprintID != sprintID {
			continue
		}
		total++
		if t.ColumnID == doneColumn {
			done++
			continue
		}
		if err := s.sprints.SetTaskSprint(ctx, orgID, t.ID, nil); err != nil {
			return nil, err
		}
	}
	now := s.now().UTC()
	sp.Status = project.SprintCompleted
	sp.EndedAt = &now
	sp.CompletedTotal = total
	sp.CompletedDone = done
	if err := s.sprints.Update(ctx, orgID, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

// DeleteSprint removes a planned sprint (LEAD). Active/completed sprints are
// history and cannot be deleted.
func (s *Service) DeleteSprint(ctx context.Context, orgID, userID, projectID, sprintID string, orgRole tenant.OrgRole) error {
	if !s.sprintsEnabled() {
		return domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	sp, err := s.sprintInProject(ctx, orgID, projectID, sprintID)
	if err != nil {
		return err
	}
	if sp.Status != project.SprintPlanned {
		return domain.ErrConflict
	}
	return s.sprints.Delete(ctx, orgID, sprintID)
}

// AssignSprint moves tasks into (nil = backlog) a sprint of their project
// (LEAD). Every task must belong to the project.
func (s *Service) AssignSprint(ctx context.Context, orgID, userID, projectID string, taskIDs []string, sprintID *string, orgRole tenant.OrgRole) error {
	if !s.sprintsEnabled() {
		return domain.ErrForbidden // sprints disabled
	}
	if _, _, err := s.access(ctx, orgID, projectID, userID, orgRole, project.RoleLead); err != nil {
		return err
	}
	if sprintID != nil {
		if _, err := s.sprintInProject(ctx, orgID, projectID, *sprintID); err != nil {
			return err
		}
	}
	if len(taskIDs) == 0 || len(taskIDs) > project.MaxBulkTasks {
		return domain.ErrValidation
	}
	for _, id := range taskIDs {
		t, err := s.tasks.Get(ctx, orgID, id)
		if err != nil {
			return err
		}
		if t.ProjectID != projectID {
			return domain.ErrValidation
		}
		if err := s.sprints.SetTaskSprint(ctx, orgID, id, sprintID); err != nil {
			return err
		}
	}
	return nil
}

// sprintInProject loads a sprint, ensuring it belongs to the project.
func (s *Service) sprintInProject(ctx context.Context, orgID, projectID, sprintID string) (*project.Sprint, error) {
	sp, err := s.sprints.Get(ctx, orgID, sprintID)
	if err != nil {
		return nil, err
	}
	if sp.ProjectID != projectID {
		return nil, domain.ErrNotFound
	}
	return sp, nil
}

// finalColumnID returns the last column by rank (the "done" bucket used for
// velocity). Empty when the board has no columns.
func (s *Service) finalColumnID(ctx context.Context, orgID, projectID string) string {
	board, err := s.boards.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return ""
	}
	cols, err := s.columns.ListByBoard(ctx, orgID, board.ID)
	if err != nil || len(cols) == 0 {
		return ""
	}
	return cols[len(cols)-1].ID // ListByBoard is rank-ordered
}
