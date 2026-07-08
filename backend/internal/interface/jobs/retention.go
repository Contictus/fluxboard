package jobs

// This file holds the audit:retention job (docs/build/PHASE-6 §7, FR-AUD-002).
// It fans out per tenant and hard-deletes audit_log rows older than the org's
// plan retention window (plans.audit_retention_days). audit_log is append-only
// for the app role (0005 REVOKE UPDATE,DELETE), so the DELETE runs on the OWNER
// pool — surfaced explicitly (worker wiring) rather than by widening the app role.

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
)

// TypeAuditRetention is the retention sweep task kind.
const TypeAuditRetention = "audit:retention"

// RetentionResolver resolves an org's entitlements (for AuditRetentionDays).
// billinguc.Service satisfies it.
type RetentionResolver interface {
	Resolve(ctx context.Context, orgID string) (billing.Entitlements, error)
}

// AuditPurger deletes an org's audit rows older than cutoff, returning the count.
// Implemented by postgres.AuditRetentionRepo over the owner pool.
type AuditPurger interface {
	DeleteOlderThan(ctx context.Context, orgID string, cutoff time.Time) (int64, error)
}

// AuditRetention runs the periodic audit-log retention sweep.
type AuditRetention struct {
	orgs   OrgLister
	ent    RetentionResolver
	purger AuditPurger // nil ⇒ owner pool unavailable; the sweep is skipped
	logger *slog.Logger
	now    func() time.Time
}

// NewAuditRetention builds the handler. A nil purger (no owner pool) makes the
// handler a logged no-op so the scheduled task never triggers a retry storm.
func NewAuditRetention(orgs OrgLister, ent RetentionResolver, purger AuditPurger, logger *slog.Logger) *AuditRetention {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditRetention{orgs: orgs, ent: ent, purger: purger, logger: logger, now: time.Now}
}

// Register wires the handler onto an Asynq mux.
func (a *AuditRetention) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeAuditRetention, a.Handle)
}

// Handle sweeps every active org, deleting audit rows past its plan retention
// window. A non-positive retention day count means "keep forever" → skip.
func (a *AuditRetention) Handle(ctx context.Context, _ *asynq.Task) error {
	if a.purger == nil {
		a.logger.Warn("audit:retention skipped: owner pool unavailable (DATABASE_URL_MIGRATE unset)")
		return nil
	}
	now := a.now().UTC()
	return forEachOrg(ctx, a.orgs, a.logger, TypeAuditRetention, func(orgID string) (int, error) {
		ent, err := a.ent.Resolve(ctx, orgID)
		if err != nil {
			return 0, err
		}
		if ent.AuditRetentionDays <= 0 {
			return 0, nil // keep forever
		}
		cutoff := now.AddDate(0, 0, -ent.AuditRetentionDays)
		n, err := a.purger.DeleteOlderThan(ctx, orgID, cutoff)
		return int(n), err
	})
}
