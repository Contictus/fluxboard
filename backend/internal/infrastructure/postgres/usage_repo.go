package postgres

import (
	"context"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// UsageRepo is the Postgres-backed billing.UsageRepository. usage_records is [T]
// (RLS), so every method runs through TenantPool.WithTenant.
type UsageRepo struct{ tp *TenantPool }

// NewUsageRepo builds a UsageRepo over the tenant pool.
func NewUsageRepo(tp *TenantPool) *UsageRepo { return &UsageRepo{tp: tp} }

var _ billing.UsageRepository = (*UsageRepo)(nil)

func (r *UsageRepo) Upsert(ctx context.Context, orgID string, rec billing.UsageRecord) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.UpsertUsage(ctx, gen.UpsertUsageParams{
			OrgID:      oid,
			Metric:     string(rec.Metric),
			PeriodDate: rec.PeriodDate,
			Value:      rec.Value,
		})
	})
}

// StorageBytes returns the org's total committed attachment bytes — the
// storage_bytes usage metric is computed straight from Postgres (SUM) at
// aggregate time rather than via a Redis gauge (deviation from 06 §5 recorded
// in docs/build/PHASE-4-BILLING.md §4.7.2: simpler and drift-free).
func (r *UsageRepo) StorageBytes(ctx context.Context, orgID string) (int64, error) {
	var n int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		var err error
		n, err = q.SumOrgAttachmentBytes(ctx, oid)
		return err
	})
	return n, err
}

func (r *UsageRepo) ListForPush(ctx context.Context, orgID string, day time.Time) ([]billing.UsageRecord, error) {
	var out []billing.UsageRecord
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListUsageForPush(ctx, gen.ListUsageForPushParams{OrgID: oid, PeriodDate: day})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, billing.UsageRecord{
				OrgID:      row.OrgID.String(),
				Metric:     billing.UsageMetric(row.Metric),
				PeriodDate: row.PeriodDate,
				Value:      row.Value,
				PushedAt:   tsPtr(row.PushedAt),
			})
		}
		return nil
	})
	return out, err
}

func (r *UsageRepo) MarkPushed(ctx context.Context, orgID string, metric billing.UsageMetric, day time.Time, at time.Time) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.MarkUsagePushed(ctx, gen.MarkUsagePushedParams{
			PushedAt:   tsVal(at),
			OrgID:      oid,
			Metric:     string(metric),
			PeriodDate: day,
		})
	})
}
