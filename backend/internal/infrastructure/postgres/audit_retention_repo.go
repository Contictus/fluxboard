package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRetentionRepo hard-deletes expired audit_log rows for the retention job
// (FR-AUD-002). audit_log is append-only for the app role (0005 REVOKE
// UPDATE,DELETE), so this repo MUST be constructed over the OWNER pool — the
// table owner bypasses the revoke. Deletion is scoped by an explicit org_id
// predicate (audit_log has no RLS); the job passes the per-plan cutoff.
type AuditRetentionRepo struct{ pool *pgxpool.Pool }

// NewAuditRetentionRepo builds the repo. Pass the OWNER pool
// (cfg.DatabaseURLMigrate) — the app pool lacks DELETE on audit_log.
func NewAuditRetentionRepo(ownerPool *pgxpool.Pool) *AuditRetentionRepo {
	return &AuditRetentionRepo{pool: ownerPool}
}

// DeleteOlderThan removes an org's audit rows created before cutoff, returning the
// number deleted.
func (r *AuditRetentionRepo) DeleteOlderThan(ctx context.Context, orgID string, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM audit_log WHERE org_id = $1::uuid AND created_at < $2`, orgID, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
