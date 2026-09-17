package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// FormRepo is the Postgres-backed project.FormRepository. project_forms is [T]
// (RLS): per-org ops run tenant-scoped via TenantPool; the public surface
// resolves the org from the token via the SECURITY DEFINER app_form_by_token
// function on the raw pool (ADR-023, same pattern as ADR-014 invitations).
type FormRepo struct {
	pool *pgxpool.Pool
	tp   *TenantPool
}

// NewFormRepo builds a FormRepo.
func NewFormRepo(pool *pgxpool.Pool, tp *TenantPool) *FormRepo {
	return &FormRepo{pool: pool, tp: tp}
}

var _ project.FormRepository = (*FormRepo)(nil)

func formFromRow(r gen.ProjectForm) *project.ProjectForm {
	return &project.ProjectForm{
		ID: r.ID.String(), OrgID: r.OrgID.String(), ProjectID: r.ProjectID.String(),
		Name: r.Name, Description: r.Description, TargetColumnID: r.TargetColumnID.String(),
		TokenHash: r.TokenHash, CreatedBy: r.CreatedBy.String(), IsActive: r.IsActive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *FormRepo) Create(ctx context.Context, orgID string, f *project.ProjectForm) error {
	id, err := parseUUID(f.ID)
	if err != nil {
		return fmt.Errorf("form create: %w", err)
	}
	pid, err := parseUUID(f.ProjectID)
	if err != nil {
		return fmt.Errorf("form create: project: %w", err)
	}
	col, err := parseUUID(f.TargetColumnID)
	if err != nil {
		return fmt.Errorf("form create: target column: %w", err)
	}
	by, err := parseUUID(f.CreatedBy)
	if err != nil {
		return fmt.Errorf("form create: created_by: %w", err)
	}
	oid, _ := parseUUID(orgID)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		return q.CreateProjectForm(ctx, gen.CreateProjectFormParams{
			ID: id, OrgID: oid, ProjectID: pid, Name: f.Name, Description: f.Description,
			TargetColumnID: col, TokenHash: f.TokenHash, CreatedBy: by,
		})
	})
	if isUnique(err) {
		return domain.ErrConflict // token hash collision (retry with a fresh token)
	}
	return err
}

func (r *FormRepo) Get(ctx context.Context, orgID, id string) (*project.ProjectForm, error) {
	formID, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("form get: %w", err)
	}
	var f *project.ProjectForm
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetProjectForm(ctx, gen.GetProjectFormParams{OrgID: oid, ID: formID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		f = formFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FormRepo) ListByProject(ctx context.Context, orgID, projectID string) ([]project.ProjectForm, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("form list: %w", err)
	}
	var out []project.ProjectForm
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListProjectFormsByProject(ctx, gen.ListProjectFormsByProjectParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, *formFromRow(row))
		}
		return nil
	})
	return out, err
}

func (r *FormRepo) Update(ctx context.Context, orgID string, f *project.ProjectForm) error {
	formID, err := parseUUID(f.ID)
	if err != nil {
		return fmt.Errorf("form update: %w", err)
	}
	col, err := parseUUID(f.TargetColumnID)
	if err != nil {
		return fmt.Errorf("form update: target column: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateProjectForm(ctx, gen.UpdateProjectFormParams{
			Name: f.Name, Description: f.Description, TargetColumnID: col,
			IsActive: f.IsActive, OrgID: oid, ID: formID,
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

func (r *FormRepo) RotateToken(ctx context.Context, orgID, id string, tokenHash []byte) error {
	formID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("form rotate: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateProjectFormToken(ctx, gen.UpdateProjectFormTokenParams{
			TokenHash: tokenHash, OrgID: oid, ID: formID,
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

func (r *FormRepo) Delete(ctx context.Context, orgID, id string) error {
	formID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("form delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteProjectForm(ctx, gen.DeleteProjectFormParams{OrgID: oid, ID: formID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ResolveByToken calls the SECURITY DEFINER function on the raw pool — the
// sanctioned cross-tenant read (ADR-023). Hand-written (sqlc can't infer the
// function's RETURNS TABLE columns).
func (r *FormRepo) ResolveByToken(ctx context.Context, tokenHash []byte) (*project.ProjectForm, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, org_id, project_id, name, description, target_column_id, created_by, is_active
		 FROM app_form_by_token($1)`, tokenHash)
	var (
		id, orgID, projectID, col, by uuid.UUID
		name, desc                    string
		active                        bool
	)
	if err := row.Scan(&id, &orgID, &projectID, &name, &desc, &col, &by, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("resolve form by token: %w", err)
	}
	return &project.ProjectForm{
		ID: id.String(), OrgID: orgID.String(), ProjectID: projectID.String(),
		Name: name, Description: desc, TargetColumnID: col.String(),
		TokenHash: tokenHash, CreatedBy: by.String(), IsActive: active,
	}, nil
}
