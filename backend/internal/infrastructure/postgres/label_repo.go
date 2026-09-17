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

// LabelRepo is the Postgres-backed project.LabelRepository ([T], RLS). Labels are
// org-scoped; task_labels is the many-to-many join.
type LabelRepo struct{ tp *TenantPool }

// NewLabelRepo builds a LabelRepo over the tenant pool.
func NewLabelRepo(tp *TenantPool) *LabelRepo { return &LabelRepo{tp: tp} }

var _ project.LabelRepository = (*LabelRepo)(nil)

func (r *LabelRepo) Create(ctx context.Context, orgID string, l *project.Label) error {
	id, err := parseUUID(l.ID)
	if err != nil {
		return fmt.Errorf("label create: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateLabel(ctx, gen.CreateLabelParams{ID: id, OrgID: oid, Name: l.Name, Color: l.Color})
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *LabelRepo) Get(ctx context.Context, orgID, id string) (*project.Label, error) {
	lid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("label get: %w", err)
	}
	var out *project.Label
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetLabel(ctx, gen.GetLabelParams{OrgID: oid, ID: lid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = &project.Label{
			ID: row.ID.String(), OrgID: row.OrgID.String(), Name: row.Name,
			Color: row.Color, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *LabelRepo) List(ctx context.Context, orgID string) ([]project.Label, error) {
	var out []project.Label
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListLabels(ctx, oid)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.Label{
				ID: row.ID.String(), OrgID: row.OrgID.String(), Name: row.Name,
				Color: row.Color, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

func (r *LabelRepo) Update(ctx context.Context, orgID, id, name, color string) error {
	lid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("label update: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateLabel(ctx, gen.UpdateLabelParams{Name: name, Color: color, OrgID: oid, ID: lid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *LabelRepo) Delete(ctx context.Context, orgID, id string) error {
	lid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("label delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteLabel(ctx, gen.DeleteLabelParams{OrgID: oid, ID: lid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *LabelRepo) Attach(ctx context.Context, orgID, taskID, labelID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("label attach: task: %w", err)
	}
	lid, err := parseUUID(labelID)
	if err != nil {
		return fmt.Errorf("label attach: label: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.AttachLabel(ctx, gen.AttachLabelParams{TaskID: tid, LabelID: lid, OrgID: oid})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound
	}
	return err
}

func (r *LabelRepo) Detach(ctx context.Context, orgID, taskID, labelID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("label detach: task: %w", err)
	}
	lid, err := parseUUID(labelID)
	if err != nil {
		return fmt.Errorf("label detach: label: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DetachLabel(ctx, gen.DetachLabelParams{OrgID: oid, TaskID: tid, LabelID: lid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *LabelRepo) ListForTask(ctx context.Context, orgID, taskID string) ([]project.Label, error) {
	tid, err := parseUUID(taskID)
	if err != nil {
		return nil, fmt.Errorf("label list for task: %w", err)
	}
	var out []project.Label
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListLabelsForTask(ctx, gen.ListLabelsForTaskParams{OrgID: oid, TaskID: tid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.Label{
				ID: row.ID.String(), OrgID: row.OrgID.String(), Name: row.Name,
				Color: row.Color, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

// ListForProject returns every label attachment in a project grouped by task
// ID, in a single query (board projection enrichment).
func (r *LabelRepo) ListForProject(ctx context.Context, orgID, projectID string) (map[string][]project.Label, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("label list for project: %w", err)
	}
	out := make(map[string][]project.Label)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListLabelsForProject(ctx, gen.ListLabelsForProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			tid := row.TaskID.String()
			out[tid] = append(out[tid], project.Label{
				ID: row.ID.String(), OrgID: row.OrgID.String(), Name: row.Name,
				Color: row.Color, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}
