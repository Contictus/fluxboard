package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

// AuditRepo is the Postgres-backed audit.Writer. audit_log is append-only for the
// app role (REVOKE UPDATE,DELETE — migration 0005) and is NOT tenant-scoped
// (org_id nullable), so it writes through the raw pool, never the TenantPool.
// Hand-written pgx rather than sqlc: the inet column and the several nullable uuid
// columns are cleaner cast in SQL than modeled through the generator.
type AuditRepo struct {
	pool *pgxpool.Pool
}

// NewAuditRepo builds an AuditRepo over the given pool.
func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

var _ audit.Writer = (*AuditRepo)(nil)

const insertAuditSQL = `
INSERT INTO audit_log
  (id, org_id, actor_user_id, impersonator_user_id, action,
   target_type, target_id, metadata, ip, user_agent, severity, created_at)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9::text::inet, $10, $11, COALESCE($12, now()))`

// Append writes one audit entry. Empty-string IDs become SQL NULL; a zero
// CreatedAt defers to the column default (now()).
func (r *AuditRepo) Append(ctx context.Context, e audit.Entry) error {
	meta := []byte("{}")
	if e.Metadata != nil {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("audit append: metadata: %w", err)
		}
		meta = b
	}
	severity := e.Severity
	if severity == "" {
		severity = audit.SeverityInfo
	}
	var createdAt *time.Time
	if !e.CreatedAt.IsZero() {
		createdAt = &e.CreatedAt
	}

	_, err := r.pool.Exec(ctx, insertAuditSQL,
		uuid.New(),                     // $1 id (audit rows get a fresh v4 id)
		nullUUID(e.OrgID),              // $2
		nullUUID(e.ActorUserID),        // $3
		nullUUID(e.ImpersonatorUserID), // $4
		string(e.Action),               // $5
		ptrOrNil(e.TargetType),         // $6
		ptrOrNil(e.TargetID),           // $7
		meta,                           // $8
		ptrOrNil(e.IP),                 // $9 ::text::inet
		ptrOrNil(e.UserAgent),          // $10
		string(severity),               // $11
		createdAt,                      // $12
	)
	if err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}

// nullUUID parses a domain id string to a uuid param, or nil for the empty
// string (→ SQL NULL). An unparseable non-empty id also maps to NULL rather than
// failing the whole audit write — the entry is best-effort telemetry.
func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return id
}
