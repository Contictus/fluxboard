package analytics

import (
	"context"
	"time"
)

// StatsRepository reads the nightly project rollup (project_stats_daily). Both are
// tenant-owned ([T]); reads run under a tenant tx (RLS, invariant #1). Analytics
// NEVER aggregates over the operational tables live — only these rollup rows.
type StatsRepository interface {
	// ListDaily returns a project's rollup rows in [from, to] (UTC days), ordered
	// by day ascending. Empty when the project has no rollup yet.
	ListDaily(ctx context.Context, orgID, projectID string, from, to time.Time) ([]DailyStat, error)
}

// UsageReader reads the org's usage aggregates (billing usage_records) for the
// dashboard. Kept separate from billing.UsageRepository (which is write/push
// oriented) so analytics stays read-only. [T]; tenant tx.
type UsageReader interface {
	// Latest returns the most recent recorded value for a metric on/before day
	// (0 when none). Used for point-in-time dimensions (seats, storage).
	Latest(ctx context.Context, orgID string, metric UsageMetric, day time.Time) (int64, error)
	// SumRange returns the total of a metric over [from, to] (UTC days). Used for
	// cumulative dimensions (api_calls).
	SumRange(ctx context.Context, orgID string, metric UsageMetric, from, to time.Time) (int64, error)
}
