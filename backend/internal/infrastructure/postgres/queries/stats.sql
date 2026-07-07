-- Project stats rollup — Phase 5 (FR-AN-001). project_stats_daily [T]: nightly
-- recompute of yesterday's grain (set-semantics UPSERT). Completion/cycle metrics
-- that need task_activity mining are deferred to Phase 6 analytics; this rollup
-- captures the created count + a current per-column open-task snapshot (recorded
-- in docs/build/PHASE-5 §4).

-- name: ListRollupProjectIDs :many
-- Non-archived, non-deleted projects for an org (rollup iteration).
SELECT id FROM projects
WHERE org_id = @org_id AND archived_at IS NULL AND deleted_at IS NULL;

-- name: CountTasksCreatedOnDay :one
-- [@day, @next_day) is the UTC day window (bounds computed by the caller).
SELECT COUNT(*) FROM tasks
WHERE org_id = @org_id AND project_id = @project_id
  AND created_at >= @day AND created_at < @next_day;

-- name: ColumnOpenCounts :many
-- Current open (non-trashed) task count per column for a project.
SELECT column_id, COUNT(*) AS cnt FROM tasks
WHERE org_id = @org_id AND project_id = @project_id AND deleted_at IS NULL
GROUP BY column_id;

-- name: UpsertProjectStat :exec
INSERT INTO project_stats_daily
  (org_id, project_id, day, created_count, completed_count, column_snapshot, avg_cycle_seconds)
VALUES (@org_id, @project_id, @day, @created_count, @completed_count, @column_snapshot, @avg_cycle_seconds)
ON CONFLICT (project_id, day) DO UPDATE SET
  created_count     = EXCLUDED.created_count,
  completed_count   = EXCLUDED.completed_count,
  column_snapshot   = EXCLUDED.column_snapshot,
  avg_cycle_seconds = EXCLUDED.avg_cycle_seconds;
