package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// OutboxRepo is the Postgres-backed billing.OutboxRepository. outbox is [T]
// (RLS). ClaimBatch runs through TenantPool.WithTenant; Insert is a package-level
// helper called inside a caller's transaction (the webhook tx).
type OutboxRepo struct{ tp *TenantPool }

// NewOutboxRepo builds an OutboxRepo over the tenant pool.
func NewOutboxRepo(tp *TenantPool) *OutboxRepo { return &OutboxRepo{tp: tp} }

var _ billing.OutboxRepository = (*OutboxRepo)(nil)

func (r *OutboxRepo) ClaimBatch(ctx context.Context, orgID string, limit int) ([]billing.OutboxItem, error) {
	var out []billing.OutboxItem
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ClaimOutboxBatch(ctx, gen.ClaimOutboxBatchParams{OrgID: oid, Lim: int32(limit)})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, billing.OutboxItem{
				ID:        row.ID.String(),
				OrgID:     row.OrgID.String(),
				Kind:      billing.OutboxKind(row.Kind),
				Payload:   row.Payload,
				CreatedAt: row.CreatedAt,
				DrainedAt: tsPtr(row.DrainedAt),
			})
		}
		return nil
	})
	return out, err
}

// InsertEmail enqueues an email:send outbox row in its own tenant tx (satisfies
// notifyuc.OutboxWriter). Notification fan-out uses this after its own writes;
// the email:send worker rechecks the recipient's preference before sending
// (09 §2/§3).
func (r *OutboxRepo) InsertEmail(ctx context.Context, orgID string, payload json.RawMessage) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return insertOutbox(ctx, q, oid, billing.OutboxItem{
			ID:      uuid.NewString(),
			OrgID:   orgID,
			Kind:    billing.OutboxEmailSend,
			Payload: payload,
		})
	})
}

// insertOutbox writes an outbox row via q (tx-bound), inside the caller's
// transaction so the enqueue is atomic with the producing write (09 §2).
func insertOutbox(ctx context.Context, q *gen.Queries, oid uuid.UUID, item billing.OutboxItem) error {
	id, err := parseUUID(item.ID)
	if err != nil {
		return fmt.Errorf("outbox insert: id: %w", err)
	}
	return q.InsertOutbox(ctx, gen.InsertOutboxParams{
		ID:      id,
		OrgID:   oid,
		Kind:    string(item.Kind),
		Payload: item.Payload,
	})
}
