package postgres

import (
	"context"

	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// FeatureFlagRepo is the Postgres-backed admin.FeatureFlagRepository. feature_flags
// is [T] (RLS via TenantPool).
type FeatureFlagRepo struct{ tp *TenantPool }

// NewFeatureFlagRepo builds the repo over the tenant pool.
func NewFeatureFlagRepo(tp *TenantPool) *FeatureFlagRepo { return &FeatureFlagRepo{tp: tp} }

var _ admin.FeatureFlagRepository = (*FeatureFlagRepo)(nil)

// List returns an org's feature flags.
func (r *FeatureFlagRepo) List(ctx context.Context, orgID string) ([]admin.FeatureFlag, error) {
	var out []admin.FeatureFlag
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListFeatureFlags(ctx, oid)
		if err != nil {
			return err
		}
		out = make([]admin.FeatureFlag, 0, len(rows))
		for _, row := range rows {
			out = append(out, admin.FeatureFlag{
				OrgID:   row.OrgID.String(),
				Flag:    row.Flag,
				Enabled: row.Enabled,
			})
		}
		return nil
	})
	return out, err
}

// Set upserts one org feature flag.
func (r *FeatureFlagRepo) Set(ctx context.Context, orgID, flag string, enabled bool) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.UpsertFeatureFlag(ctx, gen.UpsertFeatureFlagParams{
			OrgID:   oid,
			Flag:    flag,
			Enabled: enabled,
		})
	})
}
