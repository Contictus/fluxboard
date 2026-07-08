package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// AdminRepo serves the platform-admin cross-tenant reads. These span every org, so
// it runs on the OWNER pool (table owner bypasses the ENABLE (non-FORCE) RLS on
// subscriptions/memberships) — the one place that deliberately steps outside the
// per-org RLS scope, gated by the platform-admin HTTP guard.
type AdminRepo struct{ q *gen.Queries }

// NewAdminRepo builds the repo over the owner (RLS-bypassing) pool.
func NewAdminRepo(ownerPool *pgxpool.Pool) *AdminRepo { return &AdminRepo{q: gen.New(ownerPool)} }

var _ admin.AdminRepository = (*AdminRepo)(nil)

// ListTenants returns tenant summaries per the filter.
func (r *AdminRepo) ListTenants(ctx context.Context, f admin.TenantFilter) ([]admin.TenantSummary, error) {
	rows, err := r.q.ListTenants(ctx, gen.ListTenantsParams{
		Search: optStr(f.Search),
		Plan:   optStr(f.Plan),
		Status: optStr(f.Status),
		Off:    int32(f.Offset),
		Lim:    int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]admin.TenantSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, admin.TenantSummary{
			OrgID:       row.ID.String(),
			Name:        row.Name,
			Slug:        row.Slug,
			PlanCode:    row.PlanCode,
			Status:      row.Status,
			MemberCount: int(row.MemberCount),
			MRR:         row.Mrr,
			CreatedAt:   row.CreatedAt,
		})
	}
	return out, nil
}

// GetTenant returns one org's summary; domain.ErrNotFound when absent.
func (r *AdminRepo) GetTenant(ctx context.Context, orgID string) (admin.TenantSummary, error) {
	oid, err := parseUUID(orgID)
	if err != nil {
		return admin.TenantSummary{}, domain.ErrNotFound
	}
	row, err := r.q.GetTenantSummary(ctx, oid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return admin.TenantSummary{}, domain.ErrNotFound
		}
		return admin.TenantSummary{}, err
	}
	return admin.TenantSummary{
		OrgID:       row.ID.String(),
		Name:        row.Name,
		Slug:        row.Slug,
		PlanCode:    row.PlanCode,
		Status:      row.Status,
		MemberCount: int(row.MemberCount),
		MRR:         row.Mrr,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// WebhookEventsForCustomer lists processed Stripe events for a customer.
func (r *AdminRepo) WebhookEventsForCustomer(ctx context.Context, customerID string, limit int) ([]admin.WebhookEvent, error) {
	if customerID == "" {
		return nil, nil
	}
	rows, err := r.q.WebhookEventsForCustomer(ctx, gen.WebhookEventsForCustomerParams{
		CustomerID: customerID,
		Lim:        int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]admin.WebhookEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, admin.WebhookEvent{
			EventID:     row.EventID,
			Type:        row.Type,
			Handled:     row.Handled,
			Error:       row.Error,
			ProcessedAt: row.ProcessedAt,
		})
	}
	return out, nil
}

// optStr maps an empty filter string to a nil sqlc narg (SQL NULL ⇒ filter off).
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
