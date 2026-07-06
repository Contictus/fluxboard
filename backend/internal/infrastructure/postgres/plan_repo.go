package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// PlanRepo is the Postgres-backed billing.PlanRepository. plans is a GLOBAL
// reference table (no RLS), so it runs on the plain pool — no tenant context.
type PlanRepo struct{ q *gen.Queries }

// NewPlanRepo builds a PlanRepo over the plain (non-tenant) pool.
func NewPlanRepo(pool *pgxpool.Pool) *PlanRepo { return &PlanRepo{q: gen.New(pool)} }

var _ billing.PlanRepository = (*PlanRepo)(nil)

func (r *PlanRepo) Get(ctx context.Context, code billing.PlanCode) (*billing.Plan, error) {
	row, err := r.q.GetPlan(ctx, string(code))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p := planFromRow(row)
	return &p, nil
}

func (r *PlanRepo) List(ctx context.Context) ([]billing.Plan, error) {
	rows, err := r.q.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]billing.Plan, 0, len(rows))
	for _, row := range rows {
		out = append(out, planFromRow(row))
	}
	return out, nil
}

func planFromRow(row gen.Plan) billing.Plan {
	return billing.Plan{
		Code:                  billing.PlanCode(row.Code),
		Name:                  row.Name,
		StripeProductID:       derefStr(row.StripeProductID),
		SeatPriceID:           derefStr(row.SeatPriceID),
		MeteredStoragePriceID: derefStr(row.MeteredStoragePriceID),
		MeteredAPIPriceID:     derefStr(row.MeteredApiPriceID),
		MaxMembers:            int(row.MaxMembers),
		MaxProjects:           int(row.MaxProjects),
		MaxStorageBytes:       row.MaxStorageBytes,
		APIRatePerMin:         int(row.ApiRatePerMin),
		AuditRetentionDays:    int(row.AuditRetentionDays),
		Metered:               row.Metered,
	}
}

// derefStr maps a nullable text column (*string) to string ("" when SQL NULL).
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
