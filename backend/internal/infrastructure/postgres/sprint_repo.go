package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// SprintRepo is the Postgres-backed project.SprintRepository ([T], RLS).
type SprintRepo struct{ tp *TenantPool }

// NewSprintRepo builds a SprintRepo over the tenant pool.
func NewSprintRepo(tp *TenantPool) *SprintRepo { return &SprintRepo{tp: tp} }

var _ project.SprintRepository = (*SprintRepo)(nil)

func mapSprint(row gen.Sprint) *project.Sprint {
	return &project.Sprint{
		ID: row.ID.String(), OrgID: row.OrgID.String(), ProjectID: row.ProjectID.String(),
		Name: row.Name, Goal: row.Goal, Status: project.SprintStatus(row.Status),
		StartedAt: tsPtr(row.StartedAt), EndedAt: tsPtr(row.EndedAt),
		CompletedTotal: int(row.CompletedTotal), CompletedDone: int(row.CompletedDone),
		CreatedBy: row.CreatedBy.String(), CreatedAt: row.CreatedAt,
	}
}

func (r *SprintRepo) Create(ctx context.Context, orgID string, s *project.Sprint) error {
	id, err := parseUUID(s.ID)
	if err != nil {
		return fmt.Errorf("sprint create: %w", err)
	}
	pid, err := parseUUID(s.ProjectID)
	if err != nil {
		return fmt.Errorf("sprint create: project: %w", err)
	}
	by, err := parseUUID(s.CreatedBy)
	if err != nil {
		return fmt.Errorf("sprint create: by: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateSprint(ctx, gen.CreateSprintParams{
			ID: id, OrgID: oid, ProjectID: pid, Name: s.Name, Goal: s.Goal,
			Status: string(s.Status), StartedAt: nullTS(s.StartedAt), EndedAt: nullTS(s.EndedAt),
			CreatedBy: by,
		})
	})
}

func (r *SprintRepo) Get(ctx context.Context, orgID, id string) (*project.Sprint, error) {
	sid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("sprint get: %w", err)
	}
	var out *project.Sprint
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetSprint(ctx, gen.GetSprintParams{OrgID: oid, ID: sid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = mapSprint(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r *SprintRepo) ListByProject(ctx context.Context, orgID, projectID string) ([]project.Sprint, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("sprint list: %w", err)
	}
	out := make([]project.Sprint, 0)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListSprintsByProject(ctx, gen.ListSprintsByProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, *mapSprint(row))
		}
		return nil
	})
	return out, err
}

func (r *SprintRepo) GetActive(ctx context.Context, orgID, projectID string) (*project.Sprint, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("sprint active: %w", err)
	}
	var out *project.Sprint
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetActiveSprint(ctx, gen.GetActiveSprintParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = mapSprint(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r *SprintRepo) Update(ctx context.Context, orgID string, s *project.Sprint) error {
	sid, err := parseUUID(s.ID)
	if err != nil {
		return fmt.Errorf("sprint update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateSprint(ctx, gen.UpdateSprintParams{
			OrgID: oid, ID: sid, Name: s.Name, Goal: s.Goal, Status: string(s.Status),
			StartedAt: nullTS(s.StartedAt), EndedAt: nullTS(s.EndedAt),
			CompletedTotal: int32(s.CompletedTotal), CompletedDone: int32(s.CompletedDone),
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

func (r *SprintRepo) Delete(ctx context.Context, orgID, id string) error {
	sid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("sprint delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteSprint(ctx, gen.DeleteSprintParams{OrgID: oid, ID: sid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *SprintRepo) SetTaskSprint(ctx context.Context, orgID, taskID string, sprintID *string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("sprint assign: task: %w", err)
	}
	sprint, err := nullableUUID(sprintID)
	if err != nil {
		return fmt.Errorf("sprint assign: sprint: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.SetTaskSprint(ctx, gen.SetTaskSprintParams{SprintID: sprint, OrgID: oid, ID: tid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *SprintRepo) ClearSprint(ctx context.Context, orgID, projectID, sprintID string) error {
	pid, err := parseUUID(projectID)
	if err != nil {
		return fmt.Errorf("sprint clear: project: %w", err)
	}
	sid, err := parseUUID(sprintID)
	if err != nil {
		return fmt.Errorf("sprint clear: sprint: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		_, err := q.ClearProjectSprints(ctx, gen.ClearProjectSprintsParams{
			OrgID: oid, ProjectID: pid, SprintID: pgtype.UUID{Bytes: sid, Valid: true},
		})
		return err
	})
}
