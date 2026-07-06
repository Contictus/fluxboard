package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// TaskRepo is the Postgres-backed project.TaskRepository ([T], RLS).
type TaskRepo struct{ tp *TenantPool }

// NewTaskRepo builds a TaskRepo over the tenant pool.
func NewTaskRepo(tp *TenantPool) *TaskRepo { return &TaskRepo{tp: tp} }

var _ project.TaskRepository = (*TaskRepo)(nil)

// taskFrom builds a domain Task from the shared 14-column projection. sqlc emits
// a distinct row struct per query (GetTaskRow, ListTasksByColumnRow, …) because
// the explicit column list is a subset of the table, so this takes fields rather
// than one row type.
func taskFrom(
	id, orgID, projectID, columnID uuid.UUID, number int32, title, description string,
	assignee pgtype.UUID, priority string, due pgtype.Timestamptz, rankv string,
	createdBy uuid.UUID, createdAt, updatedAt time.Time,
) project.Task {
	return project.Task{
		ID: id.String(), OrgID: orgID.String(), ProjectID: projectID.String(),
		ColumnID: columnID.String(), Number: int(number), Title: title,
		Description: description, AssigneeID: uuidStrPtr(assignee),
		Priority: project.Priority(priority), DueDate: tsPtr(due),
		Rank: rankv, CreatedBy: createdBy.String(),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
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
		t := taskFrom(row.ID, row.OrgID, row.ProjectID, row.ColumnID, row.Number, row.Title,
			row.Description, row.AssigneeID, row.Priority, row.DueDate, row.Rank,
			row.CreatedBy, row.CreatedAt, row.UpdatedAt)
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
			out = append(out, taskFrom(row.ID, row.OrgID, row.ProjectID, row.ColumnID, row.Number,
				row.Title, row.Description, row.AssigneeID, row.Priority, row.DueDate, row.Rank,
				row.CreatedBy, row.CreatedAt, row.UpdatedAt))
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
			out = append(out, taskFrom(row.ID, row.OrgID, row.ProjectID, row.ColumnID, row.Number,
				row.Title, row.Description, row.AssigneeID, row.Priority, row.DueDate, row.Rank,
				row.CreatedBy, row.CreatedAt, row.UpdatedAt))
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

// ---- Trash (FR-TASK-009) --------------------------------------------------

func (r *TaskRepo) SoftDelete(ctx context.Context, orgID, id string, deletedAt time.Time) error {
	tid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("task soft-delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.SoftDeleteTask(ctx, gen.SoftDeleteTaskParams{DeletedAt: tsVal(deletedAt), OrgID: oid, ID: tid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *TaskRepo) Restore(ctx context.Context, orgID, id string) error {
	tid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("task restore: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.RestoreTask(ctx, gen.RestoreTaskParams{OrgID: oid, ID: tid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if isUnique(err) {
		return domain.ErrConflict // (column_id, rank) slot reclaimed while trashed
	}
	return err
}

func (r *TaskRepo) GetTrashed(ctx context.Context, orgID, id string) (*project.Task, error) {
	tid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("task get trashed: %w", err)
	}
	var out *project.Task
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetTrashedTask(ctx, gen.GetTrashedTaskParams{OrgID: oid, ID: tid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		t := project.Task{
			ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
			ColumnID: row.ColumnID.String(), Number: int(row.Number), Title: row.Title,
			Description: row.Description, AssigneeID: uuidStrPtr(row.AssigneeID),
			Priority: project.Priority(row.Priority), DueDate: tsPtr(row.DueDate),
			Rank: row.Rank, CreatedBy: row.CreatedBy.String(),
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		out = &t
		return nil
	})
	return out, err
}

func (r *TaskRepo) ListTrashed(ctx context.Context, orgID, projectID string) ([]project.Task, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("task list trashed: %w", err)
	}
	var out []project.Task
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListTrashedTasks(ctx, gen.ListTrashedTasksParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.Task{
				ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
				ColumnID: row.ColumnID.String(), Number: int(row.Number), Title: row.Title,
				Description: row.Description, AssigneeID: uuidStrPtr(row.AssigneeID),
				Priority: project.Priority(row.Priority), DueDate: tsPtr(row.DueDate),
				Rank: row.Rank, CreatedBy: row.CreatedBy.String(),
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

func (r *TaskRepo) PurgeExpired(ctx context.Context, orgID string, cutoff time.Time) (int, error) {
	var n int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		var err error
		n, err = q.PurgeExpiredTrash(ctx, gen.PurgeExpiredTrashParams{OrgID: oid, Cutoff: tsVal(cutoff)})
		return err
	})
	return int(n), err
}

// ---- Search (FR-TASK-007) -------------------------------------------------

func (r *TaskRepo) Search(ctx context.Context, orgID string, f project.SearchFilter) (*project.SearchResult, error) {
	oid, err := parseUUID(orgID)
	if err != nil {
		return nil, fmt.Errorf("task search: %w", err)
	}
	proj, err := optUUID(f.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("task search: project: %w", err)
	}
	assignee, err := optUUID(f.AssigneeID)
	if err != nil {
		return nil, fmt.Errorf("task search: assignee: %w", err)
	}
	col, err := optUUID(f.ColumnID)
	if err != nil {
		return nil, fmt.Errorf("task search: column: %w", err)
	}
	label, err := optUUID(f.LabelID)
	if err != nil {
		return nil, fmt.Errorf("task search: label: %w", err)
	}
	userID, err := parseUUID(f.UserID)
	if err != nil {
		return nil, fmt.Errorf("task search: user: %w", err)
	}
	out := &project.SearchResult{}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		rows, err := q.SearchTasks(ctx, gen.SearchTasksParams{
			OrgID: oid, Query: f.Query, SeeAll: f.SeeAll, UserID: userID,
			ProjectID: proj, AssigneeID: assignee, ColumnID: col,
			Priority: ptrOrNil(f.Priority), LabelID: label,
			Off: int32(f.Offset), Lim: int32(f.Limit),
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out.Total = int(row.TotalCount)
			out.Tasks = append(out.Tasks, project.Task{
				ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
				ColumnID: row.ColumnID.String(), Number: int(row.Number), Title: row.Title,
				Description: row.Description, AssigneeID: uuidStrPtr(row.AssigneeID),
				Priority: project.Priority(row.Priority), DueDate: tsPtr(row.DueDate),
				Rank: row.Rank, CreatedBy: row.CreatedBy.String(),
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

// ---- Bulk actions (FR-TASK-008) -------------------------------------------

func (r *TaskRepo) CountLive(ctx context.Context, orgID string, ids []string) (int, error) {
	uuids, err := toUUIDs(ids)
	if err != nil {
		return 0, fmt.Errorf("task count live: %w", err)
	}
	var n int64
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		var err error
		n, err = q.ValidateTaskIDs(ctx, gen.ValidateTaskIDsParams{OrgID: oid, Ids: uuids})
		return err
	})
	return int(n), err
}

func (r *TaskRepo) BulkAssign(ctx context.Context, orgID string, ids []string, assigneeID *string) error {
	uuids, err := toUUIDs(ids)
	if err != nil {
		return fmt.Errorf("task bulk assign: %w", err)
	}
	assignee, err := nullableUUID(assigneeID)
	if err != nil {
		return fmt.Errorf("task bulk assign: assignee: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		_, err := q.BulkAssignTasks(ctx, gen.BulkAssignTasksParams{AssigneeID: assignee, OrgID: oid, Ids: uuids})
		return err
	})
}

func (r *TaskRepo) BulkMove(ctx context.Context, orgID, columnID string, ids, ranks []string) error {
	if len(ids) != len(ranks) {
		return fmt.Errorf("task bulk move: ids/ranks length mismatch")
	}
	cid, err := parseUUID(columnID)
	if err != nil {
		return fmt.Errorf("task bulk move: column: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		for i, id := range ids {
			tid, err := parseUUID(id)
			if err != nil {
				return fmt.Errorf("task bulk move: id: %w", err)
			}
			n, err := q.MoveTask(ctx, gen.MoveTaskParams{ColumnID: cid, Rank: ranks[i], OrgID: oid, ID: tid})
			if err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrNotFound
			}
		}
		return nil
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

// optUUID maps an optional string id to a nullable uuid param for search facets.
func optUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

// toUUIDs parses a slice of string ids into uuid.UUIDs for array params.
func toUUIDs(ids []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, s := range ids {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}
