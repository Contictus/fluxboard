package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// TenantPool is the ONLY sanctioned way usecases touch tenant-owned ([T]) data
// (docs/05-TENANCY-RBAC.md §4). WithTenant runs fn inside a transaction whose
// app.current_tenant GUC is set to orgID, so PostgreSQL RLS scopes every query
// to that tenant — the backstop under the application-layer org_id filter
// (invariant #1). Direct pool access to tenant tables is a defect.
type TenantPool struct {
	pool *pgxpool.Pool
}

// NewTenantPool wraps a pgx pool.
func NewTenantPool(pool *pgxpool.Pool) *TenantPool { return &TenantPool{pool: pool} }

// WithTenant opens a transaction, sets app.current_tenant via set_config with
// is_local=true (SET LOCAL semantics — the GUC dies with the tx, so pooled
// connections never leak tenant context, the classic RLS-with-pooling footgun),
// then runs fn against the transaction-bound Queries. Commits on nil, rolls back
// on error.
func (p *TenantPool) WithTenant(ctx context.Context, orgID string, fn func(q *gen.Queries) error) error {
	if _, err := uuid.Parse(orgID); err != nil {
		return fmt.Errorf("tenant pool: bad org id: %w", err)
	}
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.current_tenant', $1, true)", orgID); err != nil {
			return fmt.Errorf("set tenant guc: %w", err)
		}
		return fn(gen.New(tx))
	})
}
