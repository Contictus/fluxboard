// This file is the analytics surface (FR-AN-001/002): project analytics (any
// member) and the org usage dashboard (ADMIN). Both read the nightly rollup /
// usage aggregates via analyticsuc — never live aggregation.
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/analytics"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/analyticsuc"
)

// AnalyticsHandlers serves project analytics + org usage.
type AnalyticsHandlers struct {
	svc *analyticsuc.Service
	log *slog.Logger
}

// NewAnalyticsHandlers builds AnalyticsHandlers.
func NewAnalyticsHandlers(svc *analyticsuc.Service, log *slog.Logger) *AnalyticsHandlers {
	if log == nil {
		log = slog.Default()
	}
	return &AnalyticsHandlers{svc: svc, log: log}
}

type dailyStatResp struct {
	Day             string         `json:"day"`
	CreatedCount    int            `json:"created_count"`
	CompletedCount  int            `json:"completed_count"`
	ColumnSnapshot  map[string]int `json:"column_snapshot"`
	AvgCycleSeconds *int64         `json:"avg_cycle_seconds,omitempty"`
}

// ProjectAnalytics returns a project's rollup analytics for a window.
// @Summary  Project analytics
// @Tags     analytics
// @Security BearerAuth
// @Produce  json
// @Param    orgId      path  string  true   "Organization ID"
// @Param    projectId  path  string  true   "Project ID"
// @Param    from       query string  false  "Start day (YYYY-MM-DD)"
// @Param    to         query string  false  "End day (YYYY-MM-DD)"
// @Success  200  {object}  map[string]interface{}
// @Router   /orgs/{orgId}/projects/{projectId}/analytics [get]
func (h *AnalyticsHandlers) ProjectAnalytics(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	projectID := chi.URLParam(r, "projectId")
	from, to := parseDay(r.URL.Query().Get("from")), parseDay(r.URL.Query().Get("to"))
	a, err := h.svc.ProjectAnalytics(r.Context(), tc.OrgID, projectID, from, to)
	if err != nil {
		response.Error(w, err)
		return
	}
	series := make([]dailyStatResp, 0, len(a.Series))
	for _, d := range a.Series {
		series = append(series, dailyStatResp{
			Day:             d.Day.UTC().Format("2006-01-02"),
			CreatedCount:    d.CreatedCount,
			CompletedCount:  d.CompletedCount,
			ColumnSnapshot:  d.ColumnSnapshot,
			AvgCycleSeconds: d.AvgCycleSeconds,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"project_id":        a.ProjectID,
		"from":              a.From.UTC().Format("2006-01-02"),
		"to":                a.To.UTC().Format("2006-01-02"),
		"total_created":     a.TotalCreated,
		"total_completed":   a.TotalCompleted,
		"avg_cycle_seconds": a.AvgCycleSeconds,
		"series":            series,
	})
}

// Usage returns the org usage dashboard (seats/storage/api-calls + estimate).
// @Summary  Org usage dashboard
// @Tags     analytics
// @Security BearerAuth
// @Produce  json
// @Param    orgId  path  string  true  "Organization ID"
// @Success  200  {object}  map[string]interface{}
// @Router   /orgs/{orgId}/usage [get]
func (h *AnalyticsHandlers) Usage(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	d, err := h.svc.UsageDashboard(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, usageResp(d))
}

func usageResp(d analytics.UsageDashboard) map[string]any {
	return map[string]any{
		"period_start":    d.PeriodStart.UTC().Format(time.RFC3339),
		"period_end":      d.PeriodEnd.UTC().Format(time.RFC3339),
		"seats":           d.Seats,
		"storage_bytes":   d.StorageBytes,
		"api_calls":       d.APICalls,
		"estimated_total": d.EstimatedTotal,
		"plan":            d.Plan,
	}
}

// parseDay parses a YYYY-MM-DD (or RFC3339) day; zero time when empty/invalid so the
// usecase applies its default window.
func parseDay(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
