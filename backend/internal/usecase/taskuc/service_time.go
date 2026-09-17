package taskuc

import (
	"context"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// ---- Time tracking (FR-TIME) -------------------------------------------------
// One running timer per user: starting a new timer stops the previous one so
// time is never double-counted. All writes need CONTRIBUTOR on the task's
// project; listing needs VIEWER.

// timersEnabled reports whether the time-entry repository is wired.
func (s *Service) timersEnabled() bool { return s.timeEntries != nil }

// StartTimer stops the caller's running timers and starts a new one on taskID.
func (s *Service) StartTimer(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) (*project.TimeEntry, error) {
	if !s.timersEnabled() {
		return nil, domain.ErrForbidden // timers disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if running, err := s.timeEntries.ListRunning(ctx, orgID, userID); err == nil {
		for i := range running {
			_ = s.timeEntries.Stop(ctx, orgID, running[i].ID, now)
		}
	} else {
		s.logger.Warn("timer previous-stop scan failed", "err", err, "user", userID)
	}
	e := &project.TimeEntry{
		ID: newID(), OrgID: orgID, TaskID: taskID, UserID: userID,
		StartedAt: now, CreatedAt: now,
	}
	if err := s.timeEntries.Create(ctx, orgID, e); err != nil {
		return nil, err
	}
	return e, nil
}

// StopTimer stops the caller's running entry. Only the owner may stop it.
func (s *Service) StopTimer(ctx context.Context, orgID, userID, entryID string, orgRole tenant.OrgRole) (*project.TimeEntry, error) {
	if !s.timersEnabled() {
		return nil, domain.ErrForbidden // timers disabled
	}
	e, err := s.timeEntries.Get(ctx, orgID, entryID)
	if err != nil {
		return nil, err
	}
	if e.UserID != userID {
		return nil, domain.ErrForbidden
	}
	if _, _, err := s.taskAccess(ctx, orgID, e.TaskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	if e.EndedAt != nil {
		return nil, domain.ErrConflict
	}
	if err := s.timeEntries.Stop(ctx, orgID, entryID, s.now().UTC()); err != nil {
		return nil, err
	}
	return s.timeEntries.Get(ctx, orgID, entryID)
}

// LogTimeInput carries a manual entry.
type LogTimeInput struct {
	StartedAt time.Time
	EndedAt   time.Time
	Note      string
}

// LogTime records a finished manual entry.
func (s *Service) LogTime(ctx context.Context, orgID, userID, taskID string, in LogTimeInput, orgRole tenant.OrgRole) (*project.TimeEntry, error) {
	if !s.timersEnabled() {
		return nil, domain.ErrForbidden // timers disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return nil, err
	}
	start := in.StartedAt.UTC()
	end := in.EndedAt.UTC()
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, domain.ErrValidation
	}
	if len(in.Note) > 500 {
		return nil, domain.ErrValidation
	}
	now := s.now().UTC()
	e := &project.TimeEntry{
		ID: uuidv7.New().String(), OrgID: orgID, TaskID: taskID, UserID: userID,
		StartedAt: start, EndedAt: &end, Note: in.Note, CreatedAt: now,
	}
	if err := s.timeEntries.Create(ctx, orgID, e); err != nil {
		return nil, err
	}
	return e, nil
}

// ListTimeEntries returns a task's entries newest-first (VIEWER).
func (s *Service) ListTimeEntries(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) ([]project.TimeEntry, error) {
	if !s.timersEnabled() {
		return nil, domain.ErrForbidden // timers disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	return s.timeEntries.ListByTask(ctx, orgID, taskID)
}

// DeleteTimeEntry removes an entry. Only the owner may delete it.
func (s *Service) DeleteTimeEntry(ctx context.Context, orgID, userID, entryID string, orgRole tenant.OrgRole) error {
	if !s.timersEnabled() {
		return domain.ErrForbidden // timers disabled
	}
	e, err := s.timeEntries.Get(ctx, orgID, entryID)
	if err != nil {
		return err
	}
	if e.UserID != userID {
		return domain.ErrForbidden
	}
	if _, _, err := s.taskAccess(ctx, orgID, e.TaskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	return s.timeEntries.Delete(ctx, orgID, entryID)
}
