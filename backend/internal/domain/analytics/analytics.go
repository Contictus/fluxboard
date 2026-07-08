// Package analytics is the reporting domain (docs/build/PHASE-6-ADMIN-OBS.md §2,
// FR-AN-001/002). It serves project analytics and the org usage dashboard from the
// nightly rollup (project_stats_daily) and the usage aggregates — never live
// aggregates over the operational tables. Stdlib-only.
package analytics

import "time"

// DailyStat is one project_stats_daily row (the Phase-5 rollup grain), the raw
// input the project-analytics view is assembled from.
type DailyStat struct {
	ProjectID       string
	Day             time.Time
	CreatedCount    int
	CompletedCount  int
	ColumnSnapshot  map[string]int // column_id → open task count
	AvgCycleSeconds *int64
}

// ProjectAnalytics is the assembled per-project view (FR-AN-001): the daily series
// plus roll-up totals over the requested window. CumulativeFlow is the per-day
// column snapshot series (for the CFD chart); the series is ordered by day ascending.
type ProjectAnalytics struct {
	ProjectID      string
	From           time.Time
	To             time.Time
	Series         []DailyStat
	TotalCreated   int
	TotalCompleted int
	// AvgCycleSeconds is the mean of the days that recorded one (nil ⇒ none did).
	AvgCycleSeconds *int64
}

// UsageMetric names a metered dimension mirrored from billing usage_records.
type UsageMetric string

const (
	MetricActiveMembers UsageMetric = "active_members"
	MetricStorageBytes  UsageMetric = "storage_bytes"
	MetricAPICalls      UsageMetric = "api_calls"
)

// UsageDashboard is the org usage view (FR-AN-002): the current period's metered
// dimensions plus an invoice estimate in minor units. Seats/Storage are the latest
// values; APICalls is the period sum.
type UsageDashboard struct {
	PeriodStart     time.Time
	PeriodEnd       time.Time
	Seats           int64
	StorageBytes    int64
	APICalls        int64
	EstimatedTotal  int64 // minor units (invariant #5)
	Plan            string
}
