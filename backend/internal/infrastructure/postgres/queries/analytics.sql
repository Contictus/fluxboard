-- Analytics reads — Phase 6 (FR-AN-001/002). Rollup + usage aggregates only; both
-- tables are [T] and read under a tenant tx (TenantPool). No live aggregation.

-- name: ListProjectStatsDaily :many
SELECT org_id, project_id, day, created_count, completed_count,
       column_snapshot, avg_cycle_seconds
FROM project_stats_daily
WHERE org_id = @org_id AND project_id = @project_id
  AND day >= @from_day AND day <= @to_day
ORDER BY day;

-- name: LatestUsageValue :one
-- Most recent recorded value for a metric on/before a day (point-in-time
-- dimensions: seats, storage). ErrNoRows ⇒ caller treats as 0.
SELECT value FROM usage_records
WHERE org_id = @org_id AND metric = @metric AND period_date <= @on_day
ORDER BY period_date DESC
LIMIT 1;

-- name: SumUsageRange :one
-- Total of a metric over [from, to] (cumulative dimensions: api_calls).
SELECT COALESCE(SUM(value), 0)::bigint AS total FROM usage_records
WHERE org_id = @org_id AND metric = @metric
  AND period_date >= @from_day AND period_date <= @to_day;
