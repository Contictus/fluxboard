package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// MaintenanceRepo serves cross-tenant maintenance queries that must run without a
// tenant context (organizations has no RLS). It backs the worker's periodic jobs
// (trash purge, attachment GC), which then fan out per-tenant via TenantPool.
type MaintenanceRepo struct{ q *gen.Queries }

// NewMaintenanceRepo builds a MaintenanceRepo over the plain (non-tenant) pool.
func NewMaintenanceRepo(pool *pgxpool.Pool) *MaintenanceRepo {
	return &MaintenanceRepo{q: gen.New(pool)}
}

// ListActiveOrgIDs returns every non-deleted org id.
func (r *MaintenanceRepo) ListActiveOrgIDs(ctx context.Context) ([]string, error) {
	rows, err := r.q.ListActiveOrgIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, id := range rows {
		out = append(out, id.String())
	}
	return out, nil
}
