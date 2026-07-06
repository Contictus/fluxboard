package postgres

import (
	"context"
	"fmt"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// ActivityRepo is the Postgres-backed project.ActivityRepository ([T], RLS): the
// append-only per-task change log (FR-TASK-002).
type ActivityRepo struct{ tp *TenantPool }

// NewActivityRepo builds an ActivityRepo over the tenant pool.
func NewActivityRepo(tp *TenantPool) *ActivityRepo { return &ActivityRepo{tp: tp} }

var _ project.ActivityRepository = (*ActivityRepo)(nil)

func (r *ActivityRepo) Append(ctx context.Context, orgID string, a *project.Activity) error {
	id, err := parseUUID(a.ID)
	if err != nil {
		return fmt.Errorf("activity append: id: %w", err)
	}
	tid, err := parseUUID(a.TaskID)
	if err != nil {
		return fmt.Errorf("activity append: task: %w", err)
	}
	actor, err := parseUUID(a.ActorID)
	if err != nil {
		return fmt.Errorf("activity append: actor: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.AppendActivity(ctx, gen.AppendActivityParams{
			ID: id, OrgID: oid, TaskID: tid, ActorID: actor,
			Field: a.Field, OldValue: a.OldValue, NewValue: a.NewValue,
		})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound
	}
	return err
}

func (r *ActivityRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]project.Activity, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("activity list: %w", err)
	}
	var out []project.Activity
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListActivityByTask(ctx, gen.ListActivityByTaskParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.Activity{
				ID: row.ID.String(), TaskID: row.TaskID.String(), ActorID: row.ActorID.String(),
				Field: row.Field, OldValue: row.OldValue, NewValue: row.NewValue, CreatedAt: row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}
