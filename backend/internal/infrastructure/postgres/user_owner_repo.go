package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// UserOwnerRepo answers a cross-org question — which orgs does a user solely own —
// on the OWNER pool. memberships is [T] (RLS, non-FORCE), so the table owner reads
// across tenants without a tenant context, mirroring the admin cross-org reads.
type UserOwnerRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewUserOwnerRepo builds a UserOwnerRepo over the owner pool.
func NewUserOwnerRepo(pool *pgxpool.Pool) *UserOwnerRepo {
	return &UserOwnerRepo{pool: pool, q: gen.New(pool)}
}

// SoleOwnerOrgs lists orgs where the user is the only OWNER (blocks deletion).
// Returns domain tenant.Organization values (only id/slug/name are populated) so
// this infrastructure type need not import the usecase layer.
func (r *UserOwnerRepo) SoleOwnerOrgs(ctx context.Context, userID string) ([]tenant.Organization, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("sole-owner orgs: %w", err)
	}
	rows, err := r.q.ListSoleOwnerOrgs(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("sole-owner orgs: %w", err)
	}
	out := make([]tenant.Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, tenant.Organization{ID: row.ID.String(), Slug: row.Slug, Name: row.Name})
	}
	return out, nil
}
