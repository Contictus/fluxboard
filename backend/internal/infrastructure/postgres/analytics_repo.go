package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain/analytics"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// AnalyticsRepo is the Postgres-backed analytics read layer: the project rollup
// (analytics.StatsRepository) and the usage aggregates (analytics.UsageReader).
// Both tables are [T]; every read runs through TenantPool (RLS, invariant #1).
type AnalyticsRepo struct{ tp *TenantPool }

// NewAnalyticsRepo builds the repo over the tenant pool.
func NewAnalyticsRepo(tp *TenantPool) *AnalyticsRepo { return &AnalyticsRepo{tp: tp} }

var (
	_ analytics.StatsRepository = (*AnalyticsRepo)(nil)
	_ analytics.UsageReader     = (*AnalyticsRepo)(nil)
)

// ListDaily returns a project's rollup rows in [from, to] (UTC days), day ascending.
func (r *AnalyticsRepo) ListDaily(ctx context.Context, orgID, projectID string, from, to time.Time) ([]analytics.DailyStat, error) {
	var out []analytics.DailyStat
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		pid, err := parseUUID(projectID)
		if err != nil {
			return err
		}
		rows, err := q.ListProjectStatsDaily(ctx, gen.ListProjectStatsDailyParams{
			OrgID:     oid,
			ProjectID: pid,
			FromDay:   from.UTC(),
			ToDay:     to.UTC(),
		})
		if err != nil {
			return err
		}
		out = make([]analytics.DailyStat, 0, len(rows))
		for _, row := range rows {
			snap := map[string]int{}
			if len(row.ColumnSnapshot) > 0 {
				_ = json.Unmarshal(row.ColumnSnapshot, &snap)
			}
			out = append(out, analytics.DailyStat{
				ProjectID:       row.ProjectID.String(),
				Day:             row.Day,
				CreatedCount:    int(row.CreatedCount),
				CompletedCount:  int(row.CompletedCount),
				ColumnSnapshot:  snap,
				AvgCycleSeconds: row.AvgCycleSeconds,
			})
		}
		return nil
	})
	return out, err
}

// Latest returns the most recent value for a metric on/before day (0 when none).
func (r *AnalyticsRepo) Latest(ctx context.Context, orgID string, metric analytics.UsageMetric, day time.Time) (int64, error) {
	var v int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		got, err := q.LatestUsageValue(ctx, gen.LatestUsageValueParams{
			OrgID:  oid,
			Metric: string(metric),
			OnDay:  day.UTC(),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // 0
			}
			return err
		}
		v = got
		return nil
	})
	return v, err
}

// SumRange returns the total of a metric over [from, to] (UTC days).
func (r *AnalyticsRepo) SumRange(ctx context.Context, orgID string, metric analytics.UsageMetric, from, to time.Time) (int64, error) {
	var v int64
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		got, err := q.SumUsageRange(ctx, gen.SumUsageRangeParams{
			OrgID:   oid,
			Metric:  string(metric),
			FromDay: from.UTC(),
			ToDay:   to.UTC(),
		})
		if err != nil {
			return err
		}
		v = got
		return nil
	})
	return v, err
}
