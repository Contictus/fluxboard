package taskuc

import (
	"context"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// ---- Dependencies (FR-LINKS) --------------------------------------------------
// Directed "blocked by" edges. Writes need CONTRIBUTOR on the task's project;
// the linked task must exist and be visible to the caller.

// TaskLinks bundles both directions for the detail UI.
type TaskLinks struct {
	Blockers []project.TaskLink
	Blocked  []project.TaskLink
}

// ListLinks returns both directions (VIEWER).
func (s *Service) ListLinks(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) (*TaskLinks, error) {
	if s.links == nil {
		return nil, domain.ErrForbidden // links disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleViewer); err != nil {
		return nil, err
	}
	blockers, err := s.links.Blockers(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	blocked, err := s.links.Blocked(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	return &TaskLinks{Blockers: blockers, Blocked: blocked}, nil
}

// AddLink records taskID blocked-by linkedID (CONTRIBUTOR).
func (s *Service) AddLink(ctx context.Context, orgID, userID, taskID, linkedID string, orgRole tenant.OrgRole) error {
	if s.links == nil {
		return domain.ErrForbidden // links disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	linked, err := s.tasks.Get(ctx, orgID, linkedID)
	if err != nil {
		return err
	}
	if _, err := s.access(ctx, orgID, linked.ProjectID, userID, orgRole, project.RoleViewer); err != nil {
		return err
	}
	if linked.ID == taskID {
		return domain.ErrValidation
	}
	return s.links.AddLink(ctx, orgID, taskID, linkedID)
}

// RemoveLink deletes an edge (CONTRIBUTOR).
func (s *Service) RemoveLink(ctx context.Context, orgID, userID, taskID, linkedID string, orgRole tenant.OrgRole) error {
	if s.links == nil {
		return domain.ErrForbidden // links disabled
	}
	if _, _, err := s.taskAccess(ctx, orgID, taskID, userID, orgRole, project.RoleContributor); err != nil {
		return err
	}
	return s.links.RemoveLink(ctx, orgID, taskID, linkedID)
}
