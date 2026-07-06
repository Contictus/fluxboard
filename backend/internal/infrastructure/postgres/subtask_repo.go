package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// SubtaskRepo is the Postgres-backed project.SubtaskRepository ([T], RLS).
type SubtaskRepo struct{ tp *TenantPool }

// NewSubtaskRepo builds a SubtaskRepo over the tenant pool.
func NewSubtaskRepo(tp *TenantPool) *SubtaskRepo { return &SubtaskRepo{tp: tp} }

var _ project.SubtaskRepository = (*SubtaskRepo)(nil)

func mapSubtask(row gen.Subtask) project.Subtask {
	return project.Subtask{
		ID: row.ID.String(), TaskID: row.TaskID.String(), Title: row.Title,
		Done: row.Done, Rank: row.Rank, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *SubtaskRepo) Create(ctx context.Context, orgID string, s *project.Subtask) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return fmt.Errorf("subtask create: id: %w", err)
	}
	tid, err := parseUUID(s.TaskID)
	if err != nil {
		return fmt.Errorf("subtask create: task: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateSubtask(ctx, gen.CreateSubtaskParams{
			ID: id, OrgID: oid, TaskID: tid, Title: s.Title, Done: s.Done, Rank: s.Rank,
		})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound
	}
	return err
}

func (r *SubtaskRepo) Get(ctx context.Context, orgID, id string) (*project.Subtask, error) {
	sid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("subtask get: %w", err)
	}
	var out *project.Subtask
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetSubtask(ctx, gen.GetSubtaskParams{OrgID: oid, ID: sid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		s := mapSubtask(row)
		out = &s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SubtaskRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]project.Subtask, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("subtask list: %w", err)
	}
	var out []project.Subtask
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListSubtasksByTask(ctx, gen.ListSubtasksByTaskParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapSubtask(row))
		}
		return nil
	})
	return out, err
}

func (r *SubtaskRepo) Update(ctx context.Context, orgID, id, title string, done bool) error {
	sid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("subtask update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateSubtask(ctx, gen.UpdateSubtaskParams{Title: title, Done: done, OrgID: oid, ID: sid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *SubtaskRepo) Delete(ctx context.Context, orgID, id string) error {
	sid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("subtask delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteSubtask(ctx, gen.DeleteSubtaskParams{OrgID: oid, ID: sid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
