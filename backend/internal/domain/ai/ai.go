// Package ai holds the governed AI layer domain (ADR-025, FR-AI-001..008):
// run ledger kinds, risk signal vocabulary, and the repository/provider ports.
// All tables are tenant-scoped [T] (ai_runs, ai_risks; DDL 0030, RLS 0031).
// Stdlib only. The master switch + surface toggles reuse feature_flags
// (FR-ADM-006): ai.enabled + ai.parse/ai.plan/ai.digest/ai.risk/ai.chat/ai.mcp.
package ai

import (
	"context"
	"encoding/json"
	"time"
)

// Run kinds (closed set; unknown kinds fail fast at the usecase edge).
const (
	KindParse     = "parse"
	KindPlanDraft = "plan_draft"
	KindDigest    = "digest"
	KindChat      = "chat"
	KindRisk      = "risk"
)

// ValidKind reports whether k is a known run kind.
func ValidKind(k string) bool {
	switch k {
	case KindParse, KindPlanDraft, KindDigest, KindChat, KindRisk:
		return true
	}
	return false
}

// Risk scores.
const (
	ScoreLow    = "low"
	ScoreMedium = "medium"
	ScoreHigh   = "high"
)

// ValidScore reports whether s is a known risk score.
func ValidScore(s string) bool {
	switch s {
	case ScoreLow, ScoreMedium, ScoreHigh:
		return true
	}
	return false
}

// Surface flag keys. FlagMaster is the org-level kill switch (absent ⇒ on,
// so existing orgs keep working; explicit false disables every surface).
const (
	FlagMaster = "ai.enabled"
	FlagParse  = "ai.parse"
	FlagPlan   = "ai.plan"
	FlagDigest = "ai.digest"
	FlagRisk   = "ai.risk"
	FlagChat   = "ai.chat"
	FlagMCP    = "ai.mcp"
)

// SurfaceFor maps a run kind to its surface flag.
func SurfaceFor(kind string) string {
	switch kind {
	case KindParse:
		return FlagParse
	case KindPlanDraft:
		return FlagPlan
	case KindDigest:
		return FlagDigest
	case KindRisk:
		return FlagRisk
	case KindChat:
		return FlagChat
	default:
		return ""
	}
}

// Run is one AI invocation in an organization (append-only ledger).
type Run struct {
	ID               string
	OrgID            string
	UserID           string
	Kind             string
	Input            string
	Output           json.RawMessage
	Model            string
	PromptTokens     int
	CompletionTokens int
	Status           string // ok | error
	IdempotencyKey   string
	CreatedAt        time.Time
}

// RiskSignal is one machine-readable driver behind a risk score.
type RiskSignal struct {
	Code   string `json:"code"` // overdue | overcommit | churn | scope | capacity
	Detail string `json:"detail"`
}

// Risk is one open-or-dismissed risk signal set on a task. Recompute upserts
// the open row (partial UNIQUE on (org_id, task_id) WHERE dismissed_at IS
// NULL); dismiss stamps dismissed_at so the same alert never re-noises.
type Risk struct {
	ID        string
	OrgID     string
	ProjectID string
	TaskID    string
	Score     string
	Signals   []RiskSignal
	Dismissed bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RunRepository persists the AI run ledger ([T], RLS).
type RunRepository interface {
	// Create appends a run. Duplicate (org, idempotency_key) ⇒ ErrConflict.
	Create(ctx context.Context, orgID string, r *Run) error
	// Get returns one run, or ErrNotFound (idempotency replay path).
	Get(ctx context.Context, orgID, id string) (*Run, error)
	// ListRecent returns the newest runs, bounded by limit (chat history).
	ListRecent(ctx context.Context, orgID string, limit int) ([]Run, error)
	// CountSince counts runs of any kind after t (monthly metering).
	CountSince(ctx context.Context, orgID string, t time.Time) (int64, error)
	// PurgeBefore deletes runs older than t (30d retention, owner pool job).
	PurgeBefore(ctx context.Context, orgID string, t time.Time) (int64, error)
}

// RiskRepository persists risk signals ([T], RLS).
type RiskRepository interface {
	// UpsertOpen inserts or replaces the open risk for (org, task).
	UpsertOpen(ctx context.Context, orgID string, r *Risk) error
	// ListOpen returns open risks for a project (risk board).
	ListOpen(ctx context.Context, orgID, projectID string) ([]Risk, error)
	// Dismiss stamps dismissed_at. ErrNotFound if no open risk.
	Dismiss(ctx context.Context, orgID, taskID string) error
}

// CompleteRequest is one provider call. History carries prior turns for chat;
// empty otherwise. MaxTokens bounds cost.
type CompleteRequest struct {
	Kind      string
	System    string
	User      string
	History   []string
	MaxTokens int
}

// CompleteResponse is the provider answer with usage for metering.
type CompleteResponse struct {
	Text             string
	Model            string
	PromptTokens     int
	CompletionTokens int
}

// Provider generates text. Implemented by infrastructure (mock default; live
// Claude/Azure failover later). Deterministic under the mock so tests pin it.
type Provider interface {
	Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error)
}
