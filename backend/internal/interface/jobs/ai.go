// Package jobs — AI retention (ADR-025, FR-AI-008). ai_runs input/output is
// capped at 30 days; the nightly sweep purges per tenant through the RLS
// tenant pool (aiuc.PurgeExpired). The weekly AI digest email rides the
// existing notification prefs + mailer — deferred until digest demand is
// proven (digest is on-demand via the API today).
package jobs

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TypeAIRetention purges ai_runs rows past the 30-day window (FR-AI-008).
const TypeAIRetention = "ai:retention"

// AIPurger deletes expired AI ledger rows per org. Implemented by
// aiuc.Service (PurgeExpired); declared here so jobs stays off the usecase
// concrete (same narrow-port pattern as automation.Evaluator).
type AIPurger interface {
	PurgeExpired(ctx context.Context, orgID string) (int64, error)
}

// AI runs the AI housekeeping jobs.
type AI struct {
	purger AIPurger
	orgs   OrgLister
	logger *slog.Logger
}

// NewAI builds the AI job handlers.
func NewAI(purger AIPurger, orgs OrgLister, logger *slog.Logger) *AI {
	if logger == nil {
		logger = slog.Default()
	}
	return &AI{purger: purger, orgs: orgs, logger: logger}
}

// Register wires the handlers onto an Asynq mux.
func (a *AI) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeAIRetention, a.handleRetention)
}

// handleRetention purges expired ledger rows per tenant, isolating per-org
// failures like every other sweep (09 §2).
func (a *AI) handleRetention(ctx context.Context, _ *asynq.Task) error {
	return forEachOrg(ctx, a.orgs, a.logger, TypeAIRetention, func(orgID string) (int, error) {
		n, err := a.purger.PurgeExpired(ctx, orgID)
		return int(n), err
	})
}
