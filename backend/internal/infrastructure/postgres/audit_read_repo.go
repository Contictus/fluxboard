package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

// AuditReadRepo is the Postgres-backed audit.Reader. audit_log is NOT tenant-scoped
// (nullable org_id, RLS-free), so it reads on the plain pool; org isolation is the
// explicit org_id predicate built from Filter.OrgID (the usecase forces it on the
// org-scoped path). Hand-written pgx with a dynamic WHERE — the several optional
// filters are awkward through sqlc.
type AuditReadRepo struct{ pool *pgxpool.Pool }

// NewAuditReadRepo builds the reader over the given pool.
func NewAuditReadRepo(pool *pgxpool.Pool) *AuditReadRepo { return &AuditReadRepo{pool: pool} }

var _ audit.Reader = (*AuditReadRepo)(nil)

// List returns entries newest-first per the filter, bounded by Limit.
func (r *AuditReadRepo) List(ctx context.Context, f audit.Filter) ([]audit.Entry, error) {
	var (
		conds []string
		args  []any
	)
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.OrgID != "" {
		add("org_id = $%d::uuid", f.OrgID)
	}
	if f.Actor != "" {
		add("actor_user_id = $%d::uuid", f.Actor)
	}
	if f.Action != "" {
		add("action = $%d", string(f.Action))
	}
	if f.Severity != "" {
		add("severity = $%d", string(f.Severity))
	}
	if !f.Since.IsZero() {
		add("created_at >= $%d", f.Since.UTC())
	}
	if !f.Until.IsZero() {
		add("created_at <= $%d", f.Until.UTC())
	}
	limit := f.Limit
	if limit <= 0 || limit > audit.ExportCap {
		limit = audit.ExportCap
	}

	var b strings.Builder
	b.WriteString(`SELECT org_id::text, actor_user_id::text, impersonator_user_id::text,
	                      action, target_type, target_id, metadata, host(ip), user_agent,
	                      severity, created_at
	               FROM audit_log`)
	if len(conds) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(conds, " AND "))
	}
	b.WriteString(" ORDER BY created_at DESC LIMIT " + strconv.Itoa(limit))

	rows, err := r.pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit list: %w", err)
	}
	defer rows.Close()

	var out []audit.Entry
	for rows.Next() {
		var (
			orgID, actor, impersonator *string
			targetType, targetID       *string
			ip, userAgent              *string
			meta                       []byte
			e                          audit.Entry
			action, severity           string
		)
		if err := rows.Scan(&orgID, &actor, &impersonator, &action,
			&targetType, &targetID, &meta, &ip, &userAgent, &severity, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.OrgID = derefStr(orgID)
		e.ActorUserID = derefStr(actor)
		e.ImpersonatorUserID = derefStr(impersonator)
		e.Action = audit.Action(action)
		e.TargetType = derefStr(targetType)
		e.TargetID = derefStr(targetID)
		e.IP = derefStr(ip)
		e.UserAgent = derefStr(userAgent)
		e.Severity = audit.Severity(severity)
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
