package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// StatsRepo is the Postgres-backed notify.StatsRepository, driving the nightly
// stats:rollup. project_stats_daily is [T] (RLS); all methods run through
// TenantPool.WithTenant (invariant #1).
type StatsRepo struct{ tp *TenantPool }

// NewStatsRepo builds a StatsRepo over the tenant pool.
func NewStatsRepo(tp *TenantPool) *StatsRepo { return &StatsRepo{tp: tp} }

var _ notify.StatsRepository = (*StatsRepo)(nil)

// ProjectIDs lists an org's non-archived, non-deleted project ids.
func (r *StatsRepo) ProjectIDs(ctx context.Context, orgID string) ([]string, error) {
	var out []string
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		ids, err := q.ListRollupProjectIDs(ctx, oid)
		if err != nil {
			return err
		}
		for _, id := range ids {
			out = append(out, id.String())
		}
		return nil
	})
	return out, err
}

// ComputeDay aggregates a project's activity for the given UTC day: the created
// count over [day, day+1) and a current per-column open-task snapshot. Completion
// and cycle-time metrics need task_activity mining (deferred to Phase 6), so
// CompletedCount is 0 and AvgCycleSeconds nil here.
func (r *StatsRepo) ComputeDay(ctx context.Context, orgID, projectID string, day time.Time) (notify.ProjectStat, error) {
	day = day.UTC().Truncate(24 * time.Hour)
	next := day.AddDate(0, 0, 1)
	stat := notify.ProjectStat{
		OrgID:          orgID,
		ProjectID:      projectID,
		Day:            day,
		ColumnSnapshot: map[string]int{},
	}
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		pid, err := parseUUID(projectID)
		if err != nil {
			return err
		}
		created, err := q.CountTasksCreatedOnDay(ctx, gen.CountTasksCreatedOnDayParams{
			OrgID:     oid,
			ProjectID: pid,
			Day:       day,
			NextDay:   next,
		})
		if err != nil {
			return err
		}
		stat.CreatedCount = int(created)

		cols, err := q.ColumnOpenCounts(ctx, gen.ColumnOpenCountsParams{OrgID: oid, ProjectID: pid})
		if err != nil {
			return err
		}
		for _, c := range cols {
			stat.ColumnSnapshot[c.ColumnID.String()] = int(c.Cnt)
		}
		return nil
	})
	return stat, err
}

// Upsert writes the daily rollup row (set-semantics, re-runnable).
func (r *StatsRepo) Upsert(ctx context.Context, orgID string, s notify.ProjectStat) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		pid, err := parseUUID(s.ProjectID)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(s.ColumnSnapshot)
		if err != nil {
			return err
		}
		return q.UpsertProjectStat(ctx, gen.UpsertProjectStatParams{
			OrgID:           oid,
			ProjectID:       pid,
			Day:             s.Day.UTC().Truncate(24 * time.Hour),
			CreatedCount:    int32(s.CreatedCount),
			CompletedCount:  int32(s.CompletedCount),
			ColumnSnapshot:  snapshot,
			AvgCycleSeconds: s.AvgCycleSeconds,
		})
	})
}
