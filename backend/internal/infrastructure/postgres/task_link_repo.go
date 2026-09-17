package postgres

import (
	"context"
	"fmt"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// TaskLinkRepo is the Postgres-backed project.TaskLinkRepository ([T], RLS).
type TaskLinkRepo struct{ tp *TenantPool }

// NewTaskLinkRepo builds a TaskLinkRepo over the tenant pool.
func NewTaskLinkRepo(tp *TenantPool) *TaskLinkRepo { return &TaskLinkRepo{tp: tp} }

var _ project.TaskLinkRepository = (*TaskLinkRepo)(nil)

func (r *TaskLinkRepo) AddLink(ctx context.Context, orgID, taskID, linkedID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("task link add: %w", err)
	}
	lid, err := parseUUID(linkedID)
	if err != nil {
		return fmt.Errorf("task link add: linked: %w", err)
	}
	if tid == lid {
		return domain.ErrValidation
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateTaskLink(ctx, gen.CreateTaskLinkParams{TaskID: tid, LinkedTaskID: lid, OrgID: oid})
	})
}

func (r *TaskLinkRepo) RemoveLink(ctx context.Context, orgID, taskID, linkedID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("task link remove: %w", err)
	}
	lid, err := parseUUID(linkedID)
	if err != nil {
		return fmt.Errorf("task link remove: linked: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteTaskLink(ctx, gen.DeleteTaskLinkParams{OrgID: oid, TaskID: tid, LinkedTaskID: lid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *TaskLinkRepo) list(ctx context.Context, orgID, taskID string, blockers bool) ([]project.TaskLink, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("task link list: %w", err)
	}
	out := make([]project.TaskLink, 0)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		if blockers {
			rows, err := q.ListTaskBlockers(ctx, gen.ListTaskBlockersParams{OrgID: oid, TaskID: tid})
			if err != nil {
				return err
			}
			for _, row := range rows {
				out = append(out, project.TaskLink{
					TaskID: taskID, LinkedTaskID: row.ID.String(), Title: row.Title,
					Number: int(row.Number), ColumnID: row.ColumnID.String(),
					ProjectID: row.ProjectID.String(), CreatedAt: row.CreatedAt,
				})
			}
			return nil
		}
		rows, err := q.ListTaskBlocked(ctx, gen.ListTaskBlockedParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.TaskLink{
				TaskID: row.ID.String(), LinkedTaskID: taskID, Title: row.Title,
				Number: int(row.Number), ColumnID: row.ColumnID.String(),
				ProjectID: row.ProjectID.String(), CreatedAt: row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

// Blockers returns what blocks taskID.
func (r *TaskLinkRepo) Blockers(ctx context.Context, orgID, taskID string) ([]project.TaskLink, error) {
	return r.list(ctx, orgID, taskID, true)
}

// Blocked returns what taskID blocks.
func (r *TaskLinkRepo) Blocked(ctx context.Context, orgID, taskID string) ([]project.TaskLink, error) {
	return r.list(ctx, orgID, taskID, false)
}
