package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// ProjectRepo is the Postgres-backed project.ProjectRepository. projects is [T]
// (RLS), so every method runs through TenantPool.WithTenant (invariant #1).
type ProjectRepo struct {
	tp *TenantPool
}

// NewProjectRepo builds a ProjectRepo over the tenant pool.
func NewProjectRepo(tp *TenantPool) *ProjectRepo { return &ProjectRepo{tp: tp} }

var _ project.ProjectRepository = (*ProjectRepo)(nil)

// CountByOrg returns the org's live (non-archived) project count for the
// plan-limit gate (FR-BILL-009). Concrete-only (not on ProjectRepository) —
// called by the entitlement middleware over the concrete repo.
func (r *ProjectRepo) CountByOrg(ctx context.Context, orgID string) (int64, error) {
	var n int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		c, err := q.CountProjectsByOrg(ctx, oid)
		n = c
		return err
	})
	return n, err
}

func (r *ProjectRepo) Create(ctx context.Context, orgID string, p *project.Project) error {
	id, err := parseUUID(p.ID)
	if err != nil {
		return fmt.Errorf("project create: id: %w", err)
	}
	createdBy, err := parseUUID(p.CreatedBy)
	if err != nil {
		return fmt.Errorf("project create: created_by: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateProject(ctx, gen.CreateProjectParams{
			ID: id, OrgID: oid, Key: p.Key, Name: p.Name,
			Description: p.Description, Color: p.Color,
			Visibility: string(p.Visibility), CreatedBy: createdBy,
		})
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *ProjectRepo) Get(ctx context.Context, orgID, id string) (*project.Project, error) {
	pid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("project get: %w", err)
	}
	var out *project.Project
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetProject(ctx, gen.GetProjectParams{OrgID: oid, ID: pid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = &project.Project{
			ID: row.ID.String(), OrgID: row.OrgID.String(), Key: row.Key,
			Name: row.Name, Description: row.Description, Color: row.Color,
			Visibility: project.Visibility(row.Visibility), ArchivedAt: tsPtr(row.ArchivedAt),
			CreatedBy: row.CreatedBy.String(), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProjectRepo) List(ctx context.Context, orgID, userID string, seeAll, includeArchived bool) ([]project.Project, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("project list: user: %w", err)
	}
	var out []project.Project
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListProjects(ctx, gen.ListProjectsParams{
			OrgID: oid, UserID: uid, SeeAll: seeAll, IncludeArchived: includeArchived,
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.Project{
				ID: row.ID.String(), OrgID: row.OrgID.String(), Key: row.Key,
				Name: row.Name, Description: row.Description, Color: row.Color,
				Visibility: project.Visibility(row.Visibility), ArchivedAt: tsPtr(row.ArchivedAt),
				CreatedBy: row.CreatedBy.String(), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

func (r *ProjectRepo) Update(ctx context.Context, orgID string, p *project.Project) error {
	id, err := parseUUID(p.ID)
	if err != nil {
		return fmt.Errorf("project update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateProject(ctx, gen.UpdateProjectParams{
			Name: p.Name, Description: p.Description, Color: p.Color,
			Visibility: string(p.Visibility), OrgID: oid, ID: id,
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

func (r *ProjectRepo) SetArchived(ctx context.Context, orgID, id string, archivedAt *time.Time) error {
	pid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("project archive: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.SetProjectArchived(ctx, gen.SetProjectArchivedParams{
			ArchivedAt: nullTS(archivedAt), OrgID: oid, ID: pid,
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

func (r *ProjectRepo) NextNumber(ctx context.Context, orgID, projectID string) (int, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return 0, fmt.Errorf("project next number: %w", err)
	}
	var n int
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		v, err := q.NextTaskNumber(ctx, gen.NextTaskNumberParams{OrgID: oid, ID: pid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		n = int(v)
		return nil
	})
	return n, err
}

// ProjectMemberRepo is the Postgres-backed project.ProjectMemberRepository.
type ProjectMemberRepo struct {
	tp *TenantPool
}

// NewProjectMemberRepo builds a ProjectMemberRepo over the tenant pool.
func NewProjectMemberRepo(tp *TenantPool) *ProjectMemberRepo { return &ProjectMemberRepo{tp: tp} }

var _ project.ProjectMemberRepository = (*ProjectMemberRepo)(nil)

func (r *ProjectMemberRepo) Add(ctx context.Context, orgID, projectID, userID string, role project.ProjectRole) error {
	pid, err := parseUUID(projectID)
	if err != nil {
		return fmt.Errorf("project member add: project: %w", err)
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("project member add: user: %w", err)
	}
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.AddProjectMember(ctx, gen.AddProjectMemberParams{
			ProjectID: pid, OrgID: oid, UserID: uid, Role: string(role),
		})
	})
	if isForeignKey(err) {
		return domain.ErrNotFound
	}
	return err
}

func (r *ProjectMemberRepo) Get(ctx context.Context, orgID, projectID, userID string) (*project.ProjectMember, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("project member get: project: %w", err)
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("project member get: user: %w", err)
	}
	var out *project.ProjectMember
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetProjectMember(ctx, gen.GetProjectMemberParams{OrgID: oid, ProjectID: pid, UserID: uid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = &project.ProjectMember{
			ProjectID: row.ProjectID.String(), UserID: row.UserID.String(),
			Role: project.ProjectRole(row.Role), CreatedAt: row.CreatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProjectMemberRepo) List(ctx context.Context, orgID, projectID string) ([]project.ProjectMember, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("project member list: %w", err)
	}
	var out []project.ProjectMember
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListProjectMembers(ctx, gen.ListProjectMembersParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, project.ProjectMember{
				ProjectID: row.ProjectID.String(), UserID: row.UserID.String(),
				Role: project.ProjectRole(row.Role), CreatedAt: row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

func (r *ProjectMemberRepo) Remove(ctx context.Context, orgID, projectID, userID string) error {
	pid, err := parseUUID(projectID)
	if err != nil {
		return fmt.Errorf("project member remove: project: %w", err)
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("project member remove: user: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.RemoveProjectMember(ctx, gen.RemoveProjectMemberParams{OrgID: oid, ProjectID: pid, UserID: uid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
