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

// TaskRepo is the Postgres-backed project.TaskRepository ([T], RLS).
type TaskRepo struct{ tp *TenantPool }

// NewTaskRepo builds a TaskRepo over the tenant pool.
func NewTaskRepo(tp *TenantPool) *TaskRepo { return &TaskRepo{tp: tp} }

var _ project.TaskRepository = (*TaskRepo)(nil)

func mapTask(row gen.Task) project.Task {
	return project.Task{
		ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
		ColumnID: row.ColumnID.String(), Number: int(row.Number), Title: row.Title,
		Description: row.Description, AssigneeID: uuidStrPtr(row.AssigneeID),
		Priority: project.Priority(row.Priority), DueDate: tsPtr(row.DueDate),
		Rank: row.Rank, CreatedBy: row.CreatedBy.String(),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// Create assigns the per-project task number from the project counter and
// inserts the task in one transaction (FR-TASK-001). t.Rank / t.ColumnID must be
// set by the caller.
func (r *TaskRepo) Create(ctx context.Context, orgID string, t *project.Task) error {
	id, err := parseUUID(t.ID)
	if err != nil {
		return fmt.Errorf("task create: id: %w", err)
	}
	pid, err := parseUUID(t.ProjectID)
	if err != nil {
		return fmt.Errorf("task create: project: %w", err)
	}
	cid, err := parseUUID(t.ColumnID)
	if err != nil {
		return fmt.Errorf("task create: column: %w", err)
	}
	createdBy, err := parseUUID(t.CreatedBy)
	if err != nil {
		return fmt.Errorf("task create: created_by: %w", err)
	}
	assignee, err := nullableUUID(t.AssigneeID)
	if err != nil {
		return fmt.Errorf("task create: assignee: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		num, err := q.NextTaskNumber(ctx, gen.NextTaskNumberParams{OrgID: oid, ID: pid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		t.Number = int(num)
		return q.CreateTask(ctx, gen.CreateTaskParams{
			ID: id, OrgID: oid, ProjectID: pid, ColumnID: cid, Number: num,
			Title: t.Title, Description: t.Description, AssigneeID: assignee,
			Priority: string(t.Priority), DueDate: nullTS(t.DueDate),
			Rank: t.Rank, CreatedBy: createdBy,
		})
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *TaskRepo) Get(ctx context.Context, orgID, id string) (*project.Task, error) {
	tid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("task get: %w", err)
	}
	var out *project.Task
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetTask(ctx, gen.GetTaskParams{OrgID: oid, ID: tid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		t := mapTask(row)
		out = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *TaskRepo) ListByColumn(ctx context.Context, orgID, columnID string) ([]project.Task, error) {
	cid, err := parseUUID(columnID)
	if err != nil {
		return nil, fmt.Errorf("task list column: %w", err)
	}
	var out []project.Task
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListTasksByColumn(ctx, gen.ListTasksByColumnParams{OrgID: oid, ColumnID: cid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapTask(row))
		}
		return nil
	})
	return out, err
}

func (r *TaskRepo) ListByProject(ctx context.Context, orgID, projectID string) ([]project.Task, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("task list project: %w", err)
	}
	var out []project.Task
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListTasksByProject(ctx, gen.ListTasksByProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, mapTask(row))
		}
		return nil
	})
	return out, err
}

func (r *TaskRepo) Update(ctx context.Context, orgID string, t *project.Task) error {
	tid, err := parseUUID(t.ID)
	if err != nil {
		return fmt.Errorf("task update: %w", err)
	}
	assignee, err := nullableUUID(t.AssigneeID)
	if err != nil {
		return fmt.Errorf("task update: assignee: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateTask(ctx, gen.UpdateTaskParams{
			Title: t.Title, Description: t.Description, AssigneeID: assignee,
			Priority: string(t.Priority), DueDate: nullTS(t.DueDate), OrgID: oid, ID: tid,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *TaskRepo) Move(ctx context.Context, orgID, id, columnID, rank string) error {
	tid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("task move: id: %w", err)
	}
	cid, err := parseUUID(columnID)
	if err != nil {
		return fmt.Errorf("task move: column: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.MoveTask(ctx, gen.MoveTaskParams{ColumnID: cid, Rank: rank, OrgID: oid, ID: tid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if isUnique(err) {
		return domain.ErrConflict // rank already taken in that column (concurrent move)
	}
	return err
}
