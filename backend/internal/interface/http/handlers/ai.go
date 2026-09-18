// AI surface (ADR-025, FR-AI-001..006/008). Org-scoped JSON under
// /orgs/{orgId}/ai; every call runs the usecase pipeline (surface gate →
// monthly meter → provider → ledger → audit). Auth is the standard org chain
// (HybridAuth session-or-key, TenantGuard, scope guard): API-key callers are
// attributed via ai_runs.key_id (0032), never the users FK.
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/aiuc"
)

// AIHandlers serves the governed AI surface.
type AIHandlers struct {
	svc    *aiuc.Service
	logger *slog.Logger
}

// NewAIHandlers builds AIHandlers.
func NewAIHandlers(svc *aiuc.Service, logger *slog.Logger) *AIHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &AIHandlers{svc: svc, logger: logger}
}

type parseReq struct {
	Input string `json:"input"`
}

type parseResp struct {
	RunID  string   `json:"run_id"`
	Model  string   `json:"model"`
	Tokens int      `json:"tokens"`
	Items  []string `json:"items"`
}

// Parse turns free text into task drafts (FR-AI-001).
func (h *AIHandlers) Parse(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req parseReq
	if !decodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.Parse(r.Context(), tc.OrgID, tc.UserID, req.Input)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, parseResp{
		RunID: out.RunID, Model: out.Model, Tokens: out.Tokens, Items: out.Items,
	})
}

type aiPlanReq struct {
	Brief string `json:"brief"`
}

type aiPlanResp struct {
	RunID    string   `json:"run_id"`
	Replayed bool     `json:"replayed"`
	Model    string   `json:"model"`
	Text     string   `json:"text"`
	Items    []string `json:"items"`
}

// PlanDraft turns a brief into a structured draft (FR-AI-002). The
// Idempotency-Key header is required; a retry returns the first run.
func (h *AIHandlers) PlanDraft(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req aiPlanReq
	if !decodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.PlanDraft(r.Context(), tc.OrgID, tc.UserID, req.Brief, r.Header.Get("Idempotency-Key"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, aiPlanResp{
		RunID: out.RunID, Replayed: out.Replayed, Model: out.Model,
		Text: out.Text, Items: out.Items,
	})
}

type chatReq struct {
	Message string   `json:"message"`
	History []string `json:"history"`
}

type chatResp struct {
	RunID  string `json:"run_id"`
	Model  string `json:"model"`
	Text   string `json:"text"`
	Tokens int    `json:"tokens"`
}

// Chat answers one turn with bounded history.
func (h *AIHandlers) Chat(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req chatReq
	if !decodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.Chat(r.Context(), tc.OrgID, tc.UserID, req.Message, req.History)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, chatResp{
		RunID: out.RunID, Model: out.Model, Text: out.Text, Tokens: out.Tokens,
	})
}

type digestStatsResp struct {
	ProjectID  string         `json:"project_id"`
	Total      int            `json:"total"`
	Overdue    int            `json:"overdue"`
	Unassigned int            `json:"unassigned"`
	UrgentHigh int            `json:"urgent_high"`
	ByPriority map[string]int `json:"by_priority"`
}

type digestResp struct {
	RunID string          `json:"run_id"`
	Text  string          `json:"text"`
	Stats digestStatsResp `json:"stats"`
}

// Digest builds the sponsor-ready report from live data (FR-AI-003).
func (h *AIHandlers) Digest(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	out, err := h.svc.Digest(r.Context(), tc.OrgID, tc.UserID, chi.URLParam(r, "projectId"))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, digestResp{
		RunID: out.RunID, Text: out.Text,
		Stats: digestStatsResp{
			ProjectID: out.Stats.ProjectID, Total: out.Stats.Total,
			Overdue: out.Stats.Overdue, Unassigned: out.Stats.Unassigned,
			UrgentHigh: out.Stats.UrgentHigh, ByPriority: out.Stats.ByPriority,
		},
	})
}

type riskSignalResp struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type riskResp struct {
	ID        string           `json:"id"`
	ProjectID string           `json:"project_id"`
	TaskID    string           `json:"task_id"`
	Score     string           `json:"score"`
	Signals   []riskSignalResp `json:"signals"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func toRiskResp(r *ai.Risk) riskResp {
	signals := make([]riskSignalResp, 0, len(r.Signals))
	for _, s := range r.Signals {
		signals = append(signals, riskSignalResp{Code: s.Code, Detail: s.Detail})
	}
	return riskResp{
		ID: r.ID, ProjectID: r.ProjectID, TaskID: r.TaskID, Score: r.Score,
		Signals: signals, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type scanReq struct {
	ProjectID string `json:"project_id"`
}

// ScanRisks recomputes open risks for a project (FR-AI-004, BUSINESS+).
func (h *AIHandlers) ScanRisks(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req scanReq
	if !decodeJSON(w, r, &req) {
		return
	}
	risks, err := h.svc.ScanRisks(r.Context(), tc.OrgID, tc.UserID, req.ProjectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]riskResp, 0, len(risks))
	for i := range risks {
		out = append(out, toRiskResp(&risks[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"risks": out})
}

// ListRisks returns open risks for ?project_id= (risk board).
func (h *AIHandlers) ListRisks(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	risks, err := h.svc.ListRisks(r.Context(), tc.OrgID, tc.UserID, r.URL.Query().Get("project_id"))
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]riskResp, 0, len(risks))
	for i := range risks {
		out = append(out, toRiskResp(&risks[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"risks": out})
}

type dismissReq struct {
	TaskID string `json:"task_id"`
}

// DismissRisk stamps a risk dismissed (FR-AI-004).
func (h *AIHandlers) DismissRisk(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req dismissReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.DismissRisk(r.Context(), tc.OrgID, tc.UserID, req.TaskID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
