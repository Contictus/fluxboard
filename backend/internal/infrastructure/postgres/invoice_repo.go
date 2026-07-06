package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// InvoiceRepo is the Postgres-backed billing.InvoiceRepository. invoices is [T]
// (RLS), so it runs through TenantPool.WithTenant.
type InvoiceRepo struct{ tp *TenantPool }

// NewInvoiceRepo builds an InvoiceRepo over the tenant pool.
func NewInvoiceRepo(tp *TenantPool) *InvoiceRepo { return &InvoiceRepo{tp: tp} }

var _ billing.InvoiceRepository = (*InvoiceRepo)(nil)

func (r *InvoiceRepo) Upsert(ctx context.Context, orgID string, inv *billing.Invoice) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return upsertInvoice(ctx, q, oid, inv)
	})
}

func (r *InvoiceRepo) ListByOrg(ctx context.Context, orgID string) ([]billing.Invoice, error) {
	var out []billing.Invoice
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListInvoicesByOrg(ctx, oid)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, invoiceFromRow(row))
		}
		return nil
	})
	return out, err
}

func invoiceFromRow(row gen.Invoice) billing.Invoice {
	return billing.Invoice{
		ID:              row.ID.String(),
		OrgID:           row.OrgID.String(),
		StripeInvoiceID: row.StripeInvoiceID,
		Number:          derefStr(row.Number),
		Status:          row.Status,
		AmountDue:       row.AmountDue,
		AmountPaid:      row.AmountPaid,
		Currency:        row.Currency,
		HostedPDFURL:    derefStr(row.HostedPdfUrl),
		PeriodStart:     tsPtr(row.PeriodStart),
		PeriodEnd:       tsPtr(row.PeriodEnd),
		CreatedAt:       row.CreatedAt,
	}
}

// upsertInvoice writes an invoice mirror row via q (tx-bound or pool).
func upsertInvoice(ctx context.Context, q *gen.Queries, oid uuid.UUID, inv *billing.Invoice) error {
	id, err := parseUUID(inv.ID)
	if err != nil {
		return fmt.Errorf("invoice upsert: id: %w", err)
	}
	return q.UpsertInvoice(ctx, gen.UpsertInvoiceParams{
		ID:              id,
		OrgID:           oid,
		StripeInvoiceID: inv.StripeInvoiceID,
		Number:          ptrOrNil(inv.Number),
		Status:          inv.Status,
		AmountDue:       inv.AmountDue,
		AmountPaid:      inv.AmountPaid,
		Currency:        inv.Currency,
		HostedPdfUrl:    ptrOrNil(inv.HostedPDFURL),
		PeriodStart:     nullTS(inv.PeriodStart),
		PeriodEnd:       nullTS(inv.PeriodEnd),
	})
}

// ProcessedEventRepo writes the GLOBAL webhook idempotency ledger for events
// that cannot be routed to an org (recorded handled=false). Global table, plain
// pool, no RLS.
type ProcessedEventRepo struct{ q *gen.Queries }

// NewProcessedEventRepo builds a ProcessedEventRepo over the plain pool.
func NewProcessedEventRepo(pool *pgxpool.Pool) *ProcessedEventRepo {
	return &ProcessedEventRepo{q: gen.New(pool)}
}

var _ billing.ProcessedEventRepository = (*ProcessedEventRepo)(nil)

func (r *ProcessedEventRepo) Record(ctx context.Context, e billing.ProcessedEvent) (bool, error) {
	n, err := r.q.InsertProcessedEvent(ctx, processedEventParams(e))
	if err != nil {
		return false, err
	}
	return n == 1, nil // rows-affected 0 ⇒ event_id already present (duplicate)
}

// processedEventParams maps a domain ProcessedEvent to the insert params. Shared
// with the webhook tx (webhook_repo.go).
func processedEventParams(e billing.ProcessedEvent) gen.InsertProcessedEventParams {
	return gen.InsertProcessedEventParams{
		EventID: e.EventID,
		Type:    e.Type,
		Payload: e.Payload,
		Handled: e.Handled,
		Error:   ptrOrNil(e.Error),
	}
}
