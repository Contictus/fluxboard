// Package aiuc holds the governed AI application service (ADR-025,
// FR-AI-001..008). Every provider-invoking method runs the same pipeline:
// surface gate (feature_flags, absent ⇒ on) → monthly meter (402 at cap) →
// provider complete → ledger append (ai_runs) → best-effort audit (ai.run).
// Plan drafts are idempotency-guarded (Redis primary, UNIQUE(org,key) second
// layer, same pattern as tenantuc invitations). Risk scan upserts open rows so
// recompute never re-noises the team. Depends on domain ports only.
package aiuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// CapacityLoad is the open-task count per assignee that raises a capacity
// signal on each of their tasks (FR-AI-004).
const CapacityLoad = 8

// IdempotencyTTL bounds a plan-draft replay key (same 24h as checkout/invite).
const IdempotencyTTL = 24 * time.Hour

// RetentionWindow bounds stored AI input/output (FR-AI-008, owner-pool purge).
const RetentionWindow = 30 * 24 * time.Hour

// IdempotencyStore backs the plan-draft replay guard. Implemented by
// redisx.IdempotencyStore (wired in cmd), mirroring tenantuc.
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
}

// EntitlementResolver reads the org's plan for the BUSINESS+ risk gate
// (FR-AI-004). Implemented by billinguc.Service; nil ⇒ gate skipped (tests).
type EntitlementResolver interface {
	Resolve(ctx context.Context, orgID string) (billing.Entitlements, error)
}

// Deps are the collaborators the service needs.
type Deps struct {
	Runs         ai.RunRepository
	Risks        ai.RiskRepository
	Flags        admin.FeatureFlagRepository
	Tasks        project.TaskRepository
	Projects     project.ProjectRepository
	Audit        audit.Writer
	Provider     ai.Provider
	Idem         IdempotencyStore
	Entitlements EntitlementResolver
	MonthlyCap   int // <=0 ⇒ 200
	Now          func() time.Time
	Logger       *slog.Logger
}

// Service implements the governed AI surface.
type Service struct {
	runs         ai.RunRepository
	risks        ai.RiskRepository
	flags        admin.FeatureFlagRepository
	tasks        project.TaskRepository
	projects     project.ProjectRepository
	audit        audit.Writer
	provider     ai.Provider
	idem         IdempotencyStore
	entitlements EntitlementResolver
	cap          int
	now          func() time.Time
	logger       *slog.Logger
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	cap := d.MonthlyCap
	if cap <= 0 {
		cap = 200
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		runs: d.Runs, risks: d.Risks, flags: d.Flags,
		tasks: d.Tasks, projects: d.Projects, audit: d.Audit,
		provider: d.Provider, idem: d.Idem, entitlements: d.Entitlements,
		cap: cap, now: now, logger: logger,
	}
}

// ---- outputs ---------------------------------------------------------------

// ParseOut is one NL-parse result: extracted items + ledger pointer.
type ParseOut struct {
	RunID  string
	Model  string
	Tokens int
	Items  []string
}

// PlanOut is one plan draft: prose + structured items, replay-safe.
type PlanOut struct {
	RunID    string
	Replayed bool
	Model    string
	Text     string
	Items    []string
}

// DigestStats are engine-computed numbers (never invented, FR-AI-003).
type DigestStats struct {
	ProjectID  string         `json:"project_id"`
	Total      int            `json:"total"`
	Overdue    int            `json:"overdue"`
	Unassigned int            `json:"unassigned"`
	UrgentHigh int            `json:"urgent_high"`
	ByPriority map[string]int `json:"by_priority"`
}

// DigestOut pairs the stats with the narrated prose.
type DigestOut struct {
	RunID string
	Stats DigestStats
	Text  string
}

// ChatOut is one assistant turn.
type ChatOut struct {
	RunID  string
	Model  string
	Text   string
	Tokens int
}

// ---- parse -----------------------------------------------------------------

// Parse turns free text into task drafts (FR-AI-001).
func (s *Service) Parse(ctx context.Context, orgID, userID, input string) (*ParseOut, error) {
	input = strings.TrimSpace(input)
	if input == "" || len(input) > 2000 {
		return nil, fmt.Errorf("%w: input 1..2000 chars", domain.ErrValidation)
	}
	if err := s.pipeline(ctx, orgID, ai.KindParse); err != nil {
		return nil, err
	}
	res, err := s.provider.Complete(ctx, ai.CompleteRequest{
		Kind: ai.KindParse, User: input, MaxTokens: 400,
	})
	if err != nil {
		return nil, err
	}
	run := s.ledger(ctx, orgID, userID, ai.KindParse, input, "", res)
	items := sliceLines(res.Text)
	return &ParseOut{RunID: run, Model: res.Model, Tokens: res.PromptTokens + res.CompletionTokens, Items: items}, nil
}

// ---- plan ------------------------------------------------------------------

// PlanDraft turns a brief into a structured draft (FR-AI-002). idemKey is
// required; a retry with the same key returns the first run (no duplicate).
func (s *Service) PlanDraft(ctx context.Context, orgID, userID, brief, idemKey string) (*PlanOut, error) {
	brief = strings.TrimSpace(brief)
	if brief == "" || len(brief) > 4000 {
		return nil, fmt.Errorf("%w: brief 1..4000 chars", domain.ErrValidation)
	}
	if strings.TrimSpace(idemKey) == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key required", domain.ErrValidation)
	}
	if err := s.pipeline(ctx, orgID, ai.KindPlanDraft); err != nil {
		return nil, err
	}
	key := "ai:idem:" + orgID + ":plan:" + strings.TrimSpace(idemKey)
	if s.idem != nil {
		if priorID, err := s.idem.Get(ctx, key); err == nil && priorID != "" {
			if run, err := s.runs.Get(ctx, orgID, priorID); err == nil {
				return &PlanOut{RunID: run.ID, Replayed: true, Model: run.Model, Text: runText(run), Items: sliceLines(runText(run))}, nil
			}
		}
	}
	res, err := s.provider.Complete(ctx, ai.CompleteRequest{
		Kind: ai.KindPlanDraft, User: brief, MaxTokens: 800,
	})
	if err != nil {
		return nil, err
	}
	runID := s.ledger(ctx, orgID, userID, ai.KindPlanDraft, brief, strings.TrimSpace(idemKey), res)
	if s.idem != nil {
		_ = s.idem.Set(ctx, key, runID, IdempotencyTTL)
	}
	return &PlanOut{RunID: runID, Model: res.Model, Text: res.Text, Items: sliceLines(res.Text)}, nil
}

// ---- digest ----------------------------------------------------------------

// Digest builds the sponsor-ready report from LIVE data (FR-AI-003).
func (s *Service) Digest(ctx context.Context, orgID, userID, projectID string) (*DigestOut, error) {
	p, err := s.projects.Get(ctx, orgID, projectID)
	if err != nil {
		return nil, err // ErrNotFound covers cross-tenant probes (404)
	}
	if err := s.pipeline(ctx, orgID, ai.KindDigest); err != nil {
		return nil, err
	}
	tasks, err := s.tasks.ListByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	stats := summarize(p.ID, tasks, s.now())
	statsJSON, _ := json.Marshal(stats)
	res, err := s.provider.Complete(ctx, ai.CompleteRequest{
		Kind:      ai.KindDigest,
		User:      fmt.Sprintf("project %s (%s): %s", p.Key, p.Name, string(statsJSON)),
		MaxTokens: 600,
	})
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]any{"text": res.Text, "stats": stats})
	runID := s.newRun(ctx, orgID, userID, ai.KindDigest, string(statsJSON), "", res, out)
	return &DigestOut{RunID: runID, Stats: stats, Text: res.Text}, nil
}

// ---- chat ------------------------------------------------------------------

// Chat answers one turn with bounded history (FR-AI-001/007 surface).
func (s *Service) Chat(ctx context.Context, orgID, userID, message string, history []string) (*ChatOut, error) {
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 2000 {
		return nil, fmt.Errorf("%w: message 1..2000 chars", domain.ErrValidation)
	}
	if len(history) > 10 {
		history = history[len(history)-10:]
	}
	if err := s.pipeline(ctx, orgID, ai.KindChat); err != nil {
		return nil, err
	}
	res, err := s.provider.Complete(ctx, ai.CompleteRequest{
		Kind: ai.KindChat, User: message, History: history, MaxTokens: 500,
	})
	if err != nil {
		return nil, err
	}
	runID := s.ledger(ctx, orgID, userID, ai.KindChat, message, "", res)
	return &ChatOut{RunID: runID, Model: res.Model, Text: res.Text, Tokens: res.PromptTokens + res.CompletionTokens}, nil
}

// ---- risks -----------------------------------------------------------------

// ScanRisks recomputes open risks for a project (FR-AI-004). BUSINESS+ only:
// the engine signals are cheap, the narrated review is the gated value.
func (s *Service) ScanRisks(ctx context.Context, orgID, userID, projectID string) ([]ai.Risk, error) {
	if _, err := s.projects.Get(ctx, orgID, projectID); err != nil {
		return nil, err
	}
	if err := s.pipeline(ctx, orgID, ai.KindRisk); err != nil {
		return nil, err
	}
	if err := s.requireBusiness(ctx, orgID); err != nil {
		return nil, err
	}
	tasks, err := s.tasks.ListByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	load := map[string]int{}
	for i := range tasks {
		if tasks[i].AssigneeID != nil && *tasks[i].AssigneeID != "" {
			load[*tasks[i].AssigneeID]++
		}
	}
	var out []ai.Risk
	for i := range tasks {
		t := &tasks[i]
		signals := signalsFor(t, load, now)
		if len(signals) == 0 {
			continue
		}
		risk := &ai.Risk{
			ID: uuidv7.New().String(), OrgID: orgID, ProjectID: projectID,
			TaskID: t.ID, Score: scoreFor(t, signals), Signals: signals,
		}
		if risk.Score == ai.ScoreLow {
			continue // persist signal-bearing rows only (noise budget)
		}
		if err := s.risks.UpsertOpen(ctx, orgID, risk); err != nil {
			s.logger.Warn("ai risk upsert failed", "err", err, "task", t.ID)
			continue
		}
		out = append(out, *risk)
	}
	if out == nil {
		out = []ai.Risk{}
	}
	s.writeAudit(ctx, orgID, userID, ai.KindRisk, map[string]any{"risks": len(out)})
	return out, nil
}

// ListRisks returns open risks for a project (risk board, FR-AI-004).
func (s *Service) ListRisks(ctx context.Context, orgID, _ string, projectID string) ([]ai.Risk, error) {
	if _, err := s.projects.Get(ctx, orgID, projectID); err != nil {
		return nil, err
	}
	risks, err := s.risks.ListOpen(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if risks == nil {
		risks = []ai.Risk{}
	}
	return risks, nil
}

// DismissRisk stamps a risk dismissed (FR-AI-004). The task read first proves
// org visibility, so dismissing a foreign task 404s instead of leaking.
func (s *Service) DismissRisk(ctx context.Context, orgID, userID, taskID string) error {
	if _, err := s.tasks.Get(ctx, orgID, taskID); err != nil {
		return err
	}
	if err := s.risks.Dismiss(ctx, orgID, taskID); err != nil {
		return err
	}
	s.writeAudit(ctx, orgID, userID, "risk_dismiss", map[string]any{"task_id": taskID})
	return nil
}

// PurgeExpired deletes ledger rows past retention (job path, FR-AI-008).
func (s *Service) PurgeExpired(ctx context.Context, orgID string) (int64, error) {
	return s.runs.PurgeBefore(ctx, orgID, s.now().Add(-RetentionWindow))
}

// ---- pipeline ---------------------------------------------------------------

// pipeline runs the per-call gates: surface flag then monthly meter.
func (s *Service) pipeline(ctx context.Context, orgID, kind string) error {
	if err := s.gate(ctx, orgID, kind); err != nil {
		return err
	}
	n, err := s.runs.CountSince(ctx, orgID, monthStart(s.now()))
	if err != nil {
		return err
	}
	if n >= int64(s.cap) {
		return fmt.Errorf("%w: monthly AI budget used", domain.ErrPlanLimit)
	}
	return nil
}

// gate enforces the master switch + surface flag. Absent rows ⇒ on (existing
// orgs keep working); explicit false ⇒ 403. Server-side: the handler has no
// bypass, so a direct API call honors the same gate (FR-AI-005).
func (s *Service) gate(ctx context.Context, orgID, kind string) error {
	if s.flags == nil {
		return nil
	}
	rows, err := s.flags.List(ctx, orgID)
	if err != nil {
		return err
	}
	m := make(map[string]bool, len(rows))
	for _, f := range rows {
		m[f.Flag] = f.Enabled
	}
	if v, ok := m[ai.FlagMaster]; ok && !v {
		return fmt.Errorf("%w: ai disabled for this org", domain.ErrForbidden)
	}
	if flag := ai.SurfaceFor(kind); flag != "" {
		if v, ok := m[flag]; ok && !v {
			return fmt.Errorf("%w: ai surface disabled", domain.ErrForbidden)
		}
	}
	return nil
}

// requireBusiness gates the narrated risk review on the business tier
// (FR-AI-004). Nil resolver ⇒ skipped (tests without billing).
func (s *Service) requireBusiness(ctx context.Context, orgID string) error {
	if s.entitlements == nil {
		return nil
	}
	ent, err := s.entitlements.Resolve(ctx, orgID)
	if err != nil {
		return err
	}
	if ent.Plan != billing.PlanBusiness {
		return fmt.Errorf("%w: risk review needs the business plan", domain.ErrPlanLimit)
	}
	return nil
}

// ledger persists the run + audits it. Audit is best-effort (never fails the
// call); a failed ledger append surfaces (the call produced billable work).
func (s *Service) ledger(ctx context.Context, orgID, userID, kind, input, idemKey string, res CompleteResponseAlias) string {
	out, _ := json.Marshal(map[string]string{"text": res.Text})
	return s.newRun(ctx, orgID, userID, kind, input, idemKey, res, out)
}

// newRun stores one ledger row and audits it best-effort.
func (s *Service) newRun(ctx context.Context, orgID, userID, kind, input, idemKey string, res CompleteResponseAlias, out json.RawMessage) string {
	run := &ai.Run{
		ID: uuidv7.New().String(), OrgID: orgID, UserID: userID,
		Kind: kind, Input: input, Output: out, Model: res.Model,
		PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
		Status: "ok", IdempotencyKey: idemKey,
	}
	if err := s.runs.Create(ctx, orgID, run); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return run.ID // idempotency race lost; the winner row holds the work
		}
		s.logger.Warn("ai ledger append failed", "err", err, "org", orgID, "kind", kind)
	}
	s.writeAudit(ctx, orgID, userID, kind, map[string]any{
		"run_id": run.ID, "model": res.Model,
		"tokens": res.PromptTokens + res.CompletionTokens,
	})
	return run.ID
}

// writeAudit appends best-effort (a failed append is logged, never surfaced).
func (s *Service) writeAudit(ctx context.Context, orgID, userID, kind string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	action := audit.ActionAIRun
	if kind == "risk_dismiss" {
		action = audit.ActionAIRiskDismiss
	}
	if err := s.audit.Append(ctx, audit.Entry{
		OrgID: orgID, ActorUserID: userID, Action: action,
		TargetType: "ai_run", Metadata: meta, Severity: audit.SeverityInfo,
	}); err != nil {
		s.logger.Warn("ai audit append failed", "err", err, "org", orgID)
	}
}

// ---- engine (no invented numbers) -------------------------------------------

// summarize computes digest stats from live rows only (FR-AI-003).
func summarize(projectID string, tasks []project.Task, now time.Time) DigestStats {
	st := DigestStats{ProjectID: projectID, ByPriority: map[string]int{}}
	for i := range tasks {
		t := &tasks[i]
		st.Total++
		st.ByPriority[string(t.Priority)]++
		if t.Priority == project.PriorityUrgent || t.Priority == project.PriorityHigh {
			st.UrgentHigh++
		}
		if t.AssigneeID == nil || *t.AssigneeID == "" {
			st.Unassigned++
		}
		if t.DueDate != nil && t.DueDate.Before(now) {
			st.Overdue++
		}
	}
	return st
}

// signalsFor derives machine-readable risk drivers for one task.
func signalsFor(t *project.Task, load map[string]int, now time.Time) []ai.RiskSignal {
	var out []ai.RiskSignal
	if t.DueDate != nil && t.DueDate.Before(now) {
		out = append(out, ai.RiskSignal{Code: "overdue", Detail: "past due date"})
	}
	hot := t.Priority == project.PriorityUrgent || t.Priority == project.PriorityHigh
	if (t.AssigneeID == nil || *t.AssigneeID == "") && hot {
		out = append(out, ai.RiskSignal{Code: "unassigned_hot", Detail: "urgent/high with no owner"})
	}
	if t.AssigneeID != nil && load[*t.AssigneeID] > CapacityLoad {
		out = append(out, ai.RiskSignal{
			Code:   "capacity",
			Detail: fmt.Sprintf("owner carries %d open tasks", load[*t.AssigneeID]),
		})
	}
	return out
}

// scoreFor maps signals to a score. Overdue heat ⇒ high; any signal ⇒ medium.
func scoreFor(t *project.Task, signals []ai.RiskSignal) string {
	hot := t.Priority == project.PriorityUrgent || t.Priority == project.PriorityHigh
	for _, sg := range signals {
		if sg.Code == "overdue" && hot {
			return ai.ScoreHigh
		}
	}
	if len(signals) >= 3 {
		return ai.ScoreHigh
	}
	if len(signals) > 0 {
		return ai.ScoreMedium
	}
	return ai.ScoreLow
}

// sliceLines extracts "- item" lines from mock/provider prose.
func sliceLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			if item := strings.TrimSpace(strings.TrimPrefix(line, "- ")); item != "" {
				out = append(out, item)
			}
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// runText pulls the prose back out of a stored run (replay path).
func runText(run *ai.Run) string {
	var doc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(run.Output, &doc); err == nil && doc.Text != "" {
		return doc.Text
	}
	return string(run.Output)
}

// monthStart is the UTC month floor for metering (FR-AI-008).
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// CompleteResponseAlias keeps the ledger helper independent of the provider
// import cycle surface (same shape as ai.CompleteResponse).
type CompleteResponseAlias = ai.CompleteResponse
