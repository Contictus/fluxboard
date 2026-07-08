package postgres

import (
	"context"

	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// OverrideRepo is the Postgres-backed admin.OverrideRepository. entitlement_overrides
// is [T] (RLS via TenantPool).
type OverrideRepo struct{ tp *TenantPool }

// NewOverrideRepo builds the repo over the tenant pool.
func NewOverrideRepo(tp *TenantPool) *OverrideRepo { return &OverrideRepo{tp: tp} }

var _ admin.OverrideRepository = (*OverrideRepo)(nil)

// List returns an org's entitlement overrides.
func (r *OverrideRepo) List(ctx context.Context, orgID string) ([]admin.EntitlementOverride, error) {
	var out []admin.EntitlementOverride
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListOverrides(ctx, oid)
		if err != nil {
			return err
		}
		out = make([]admin.EntitlementOverride, 0, len(rows))
		for _, row := range rows {
			out = append(out, admin.EntitlementOverride{
				OrgID:     row.OrgID.String(),
				Key:       row.Key,
				Value:     row.Value,
				Note:      row.Note,
				CreatedBy: derefStr(uuidStrPtr(row.CreatedBy)),
				CreatedAt: row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

// Set upserts one entitlement override.
func (r *OverrideRepo) Set(ctx context.Context, o admin.EntitlementOverride) error {
	return r.tp.WithTenant(ctx, o.OrgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(o.OrgID)
		createdBy, err := nullableUUID(ptrOrNil(o.CreatedBy))
		if err != nil {
			return err
		}
		return q.UpsertOverride(ctx, gen.UpsertOverrideParams{
			OrgID:     oid,
			Key:       o.Key,
			Value:     o.Value,
			Note:      o.Note,
			CreatedBy: createdBy,
			CreatedAt: o.CreatedAt,
		})
	})
}

// Delete removes one entitlement override.
func (r *OverrideRepo) Delete(ctx context.Context, orgID, key string) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		_, err := q.DeleteOverride(ctx, gen.DeleteOverrideParams{OrgID: oid, Key: key})
		return err
	})
}
