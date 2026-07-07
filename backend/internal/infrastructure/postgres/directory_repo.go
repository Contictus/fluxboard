package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// DirUser is a directory entry (id/email/name) for notification fan-out. It is a
// postgres-layer DTO; the cmd wiring adapts it to notifyuc.Directory so this
// package stays free of a usecase import (clean-arch layering).
type DirUser struct {
	ID    string
	Email string
	Name  string
}

// DirectoryRepo resolves project membership and user contact info for the Phase-5
// notification fan-out. project_members is [T] (RLS); users is global. Both reads
// run inside a tenant tx via TenantPool.WithTenant.
type DirectoryRepo struct{ tp *TenantPool }

// NewDirectoryRepo builds a DirectoryRepo over the tenant pool.
func NewDirectoryRepo(tp *TenantPool) *DirectoryRepo { return &DirectoryRepo{tp: tp} }

// ProjectMembers returns the members of a project (for @mention matching).
func (r *DirectoryRepo) ProjectMembers(ctx context.Context, orgID, projectID string) ([]DirUser, error) {
	var out []DirUser
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		pid, err := parseUUID(projectID)
		if err != nil {
			return err
		}
		rows, err := q.ListProjectMemberUsers(ctx, gen.ListProjectMemberUsersParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, DirUser{ID: row.ID.String(), Email: row.Email, Name: row.Name})
		}
		return nil
	})
	return out, err
}

// UsersByID resolves contact info for specific user ids (targeted notifs). An
// empty id list is a no-op.
func (r *DirectoryRepo) UsersByID(ctx context.Context, orgID string, ids []string) ([]DirUser, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []DirUser
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		uids := make([]uuid.UUID, 0, len(ids))
		for _, id := range ids {
			u, err := parseUUID(id)
			if err != nil {
				return err
			}
			uids = append(uids, u)
		}
		rows, err := q.ListUsersByIDs(ctx, uids)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, DirUser{ID: row.ID.String(), Email: row.Email, Name: row.Name})
		}
		return nil
	})
	return out, err
}
