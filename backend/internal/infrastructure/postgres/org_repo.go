package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// OrgRepo is the Postgres-backed tenant.OrgRepository. organizations has no RLS
// (it must resolve before tenant context exists), so reads/writes use the raw
// pool. The "my orgs" cross-tenant read goes through the SECURITY DEFINER
// app_current_user_orgs function (docs/05 §4, ADR-014).
type OrgRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewOrgRepo builds an OrgRepo over the given pool.
func NewOrgRepo(pool *pgxpool.Pool) *OrgRepo {
	return &OrgRepo{pool: pool, q: gen.New(pool)}
}

var _ tenant.OrgRepository = (*OrgRepo)(nil)

func (r *OrgRepo) CreateWithOwner(ctx context.Context, o *tenant.Organization, ownerUserID string) error {
	orgID, err := parseUUID(o.ID)
	if err != nil {
		return fmt.Errorf("org create: %w", err)
	}
	ownerID, err := parseUUID(ownerUserID)
	if err != nil {
		return fmt.Errorf("org create: owner id: %w", err)
	}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := q.CreateOrg(ctx, gen.CreateOrgParams{ID: orgID, Slug: o.Slug, Name: o.Name}); err != nil {
			return err
		}
		// Set tenant context so the OWNER membership insert satisfies RLS in the
		// same transaction as the org row.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", o.ID); err != nil {
			return err
		}
		return q.CreateMembership(ctx, gen.CreateMembershipParams{
			OrgID: orgID, UserID: ownerID, Role: string(tenant.RoleOwner),
		})
	})
	if isUnique(err) {
		return domain.ErrConflict // slug already taken
	}
	return err
}

func (r *OrgRepo) GetByID(ctx context.Context, id string) (*tenant.Organization, error) {
	oid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("org by id: %w", err)
	}
	row, err := r.q.GetOrgByID(ctx, oid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("org by id: %w", err)
	}
	return &tenant.Organization{
		ID: row.ID.String(), Slug: row.Slug, Name: row.Name,
		LogoKey: row.LogoKey, StripeCustomerID: row.StripeCustomerID,
		DeletedAt: tsPtr(row.DeletedAt), PurgeAfter: tsPtr(row.PurgeAfter),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *OrgRepo) GetBySlug(ctx context.Context, slug string) (*tenant.Organization, error) {
	row, err := r.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("org by slug: %w", err)
	}
	return &tenant.Organization{
		ID: row.ID.String(), Slug: row.Slug, Name: row.Name,
		LogoKey: row.LogoKey, StripeCustomerID: row.StripeCustomerID,
		DeletedAt: tsPtr(row.DeletedAt), PurgeAfter: tsPtr(row.PurgeAfter),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// ListForUser calls the SECURITY DEFINER function directly (raw pool) — it is
// the sanctioned cross-tenant read (ADR-014). Hand-written because sqlc cannot
// infer the function's RETURNS TABLE columns.
func (r *OrgRepo) ListForUser(ctx context.Context, userID string) ([]tenant.OrgMembership, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT org_id, role, slug, name, created_at FROM app_current_user_orgs($1)`, uid)
	if err != nil {
		return nil, fmt.Errorf("list user orgs: %w", err)
	}
	defer rows.Close()
	var out []tenant.OrgMembership
	for rows.Next() {
		var m tenant.OrgMembership
		var orgID uuid.UUID
		var role string
		if err := rows.Scan(&orgID, &role, &m.Slug, &m.Name, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("list user orgs scan: %w", err)
		}
		m.OrgID = orgID.String()
		m.Role = tenant.OrgRole(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *OrgRepo) UpdateProfile(ctx context.Context, id, name string, logoKey *string) error {
	oid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("update org profile: %w", err)
	}
	return r.q.UpdateOrgProfile(ctx, gen.UpdateOrgProfileParams{Name: name, LogoKey: logoKey, ID: oid})
}

func (r *OrgRepo) UpdateSlug(ctx context.Context, id, newSlug string, historyExpires time.Time) error {
	oid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("update org slug: %w", err)
	}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.GetOrgByID(ctx, oid)
		if err != nil {
			return err
		}
		if err := q.UpdateOrgSlug(ctx, gen.UpdateOrgSlugParams{Slug: newSlug, ID: oid}); err != nil {
			return err
		}
		// Record the old slug for the 301 window (FR-TEN-007).
		return q.InsertSlugHistory(ctx, gen.InsertSlugHistoryParams{
			OldSlug: cur.Slug, OrgID: oid, ExpiresAt: historyExpires,
		})
	})
	if isUnique(err) {
		return domain.ErrConflict // slug already taken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func (r *OrgRepo) SoftDelete(ctx context.Context, id string, purgeAfter time.Time) error {
	oid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("soft delete org: %w", err)
	}
	return r.q.SoftDeleteOrg(ctx, gen.SoftDeleteOrgParams{PurgeAfter: tsVal(purgeAfter), ID: oid})
}

func (r *OrgRepo) Restore(ctx context.Context, id string) error {
	oid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("restore org: %w", err)
	}
	return r.q.RestoreOrg(ctx, oid)
}
