package analyticsuc

import (
	"context"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain/analytics"
)

type fakeStats struct {
	rows      []analytics.DailyStat
	gotFrom   time.Time
	gotTo     time.Time
}

func (f *fakeStats) ListDaily(_ context.Context, _, _ string, from, to time.Time) ([]analytics.DailyStat, error) {
	f.gotFrom, f.gotTo = from, to
	return f.rows, nil
}

func i64(v int64) *int64 { return &v }

// ProjectAnalytics totals + mean cycle must equal a raw recompute over the rollup.
func TestProjectAnalyticsMatchesRawRecompute(t *testing.T) {
	rows := []analytics.DailyStat{
		{CreatedCount: 3, CompletedCount: 1, AvgCycleSeconds: i64(100)},
		{CreatedCount: 2, CompletedCount: 4, AvgCycleSeconds: nil}, // a day with no completions → no cycle
		{CreatedCount: 5, CompletedCount: 2, AvgCycleSeconds: i64(300)},
	}
	stats := &fakeStats{rows: rows}
	svc := New(Deps{Stats: stats})

	out, err := svc.ProjectAnalytics(context.Background(), "org1", "proj1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}

	// Raw recompute.
	var wantCreated, wantCompleted int
	var cycleSum, cycleDays int64
	for _, d := range rows {
		wantCreated += d.CreatedCount
		wantCompleted += d.CompletedCount
		if d.AvgCycleSeconds != nil {
			cycleSum += *d.AvgCycleSeconds
			cycleDays++
		}
	}
	if out.TotalCreated != wantCreated {
		t.Fatalf("TotalCreated = %d, want %d", out.TotalCreated, wantCreated)
	}
	if out.TotalCompleted != wantCompleted {
		t.Fatalf("TotalCompleted = %d, want %d", out.TotalCompleted, wantCompleted)
	}
	if out.AvgCycleSeconds == nil {
		t.Fatal("AvgCycleSeconds nil, want mean over recording days")
	}
	if want := cycleSum / cycleDays; *out.AvgCycleSeconds != want {
		t.Fatalf("AvgCycleSeconds = %d, want %d", *out.AvgCycleSeconds, want)
	}
	if len(out.Series) != len(rows) {
		t.Fatalf("Series len = %d, want %d", len(out.Series), len(rows))
	}
}

// A zero from/to defaults to the last 30 UTC days (truncated to day boundaries).
func TestProjectAnalyticsDefaultWindow(t *testing.T) {
	stats := &fakeStats{}
	now := time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC)
	svc := New(Deps{Stats: stats, Now: func() time.Time { return now }})

	if _, err := svc.ProjectAnalytics(context.Background(), "org1", "proj1", time.Time{}, time.Time{}); err != nil {
		t.Fatalf("analytics: %v", err)
	}
	wantTo := now.Truncate(24 * time.Hour)
	wantFrom := now.Add(-defaultWindow).Truncate(24 * time.Hour)
	if !stats.gotTo.Equal(wantTo) {
		t.Fatalf("to = %v, want %v", stats.gotTo, wantTo)
	}
	if !stats.gotFrom.Equal(wantFrom) {
		t.Fatalf("from = %v, want %v", stats.gotFrom, wantFrom)
	}
}

// No rollup rows → zero totals and a nil mean (nothing recorded a cycle).
func TestProjectAnalyticsEmpty(t *testing.T) {
	svc := New(Deps{Stats: &fakeStats{rows: nil}})
	out, err := svc.ProjectAnalytics(context.Background(), "org1", "proj1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if out.TotalCreated != 0 || out.TotalCompleted != 0 || out.AvgCycleSeconds != nil {
		t.Fatalf("empty analytics not zero-valued: %+v", out)
	}
}
