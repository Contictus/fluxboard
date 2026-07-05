package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// InvitationRepo is the Postgres-backed tenant.InvitationRepository. invitations
// is [T] (RLS): per-org ops run tenant-scoped via TenantPool; the accept flow
// resolves the org from the token via the SECURITY DEFINER
// app_invitation_by_token function on the raw pool (ADR-014).
type InvitationRepo struct {
	pool *pgxpool.Pool
	tp   *TenantPool
}

// NewInvitationRepo builds an InvitationRepo.
func NewInvitationRepo(pool *pgxpool.Pool, tp *TenantPool) *InvitationRepo {
	return &InvitationRepo{pool: pool, tp: tp}
}

var _ tenant.InvitationRepository = (*InvitationRepo)(nil)

func invFromRow(r gen.Invitation) *tenant.Invitation {
	return &tenant.Invitation{
		ID: r.ID.String(), OrgID: r.OrgID.String(), Email: r.Email,
		Role: tenant.OrgRole(r.Role), TokenHash: r.TokenHash,
		InvitedBy: r.InvitedBy.String(), ExpiresAt: r.ExpiresAt,
		AcceptedAt: tsPtr(r.AcceptedAt), RevokedAt: tsPtr(r.RevokedAt),
		CreatedAt: r.CreatedAt,
	}
}

func (r *InvitationRepo) Create(ctx context.Context, inv *tenant.Invitation) error {
	id, err := parseUUID(inv.ID)
	if err != nil {
		return fmt.Errorf("invite create: %w", err)
	}
	invitedBy, err := parseUUID(inv.InvitedBy)
	if err != nil {
		return fmt.Errorf("invite create: invited_by: %w", err)
	}
	oid, _ := parseUUID(inv.OrgID)
	err = r.tp.WithTenant(ctx, inv.OrgID, func(q *gen.Queries) error {
		return q.CreateInvitation(ctx, gen.CreateInvitationParams{
			ID: id, OrgID: oid, Email: inv.Email, Role: string(inv.Role),
			TokenHash: inv.TokenHash, InvitedBy: invitedBy, ExpiresAt: inv.ExpiresAt,
		})
	})
	if isUnique(err) {
		return domain.ErrConflict // pending invite already exists for this email
	}
	return err
}

func (r *InvitationRepo) Get(ctx context.Context, orgID, id string) (*tenant.Invitation, error) {
	invID, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invite get: %w", err)
	}
	var inv *tenant.Invitation
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetInvitation(ctx, gen.GetInvitationParams{OrgID: oid, ID: invID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		inv = invFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *InvitationRepo) GetByEmail(ctx context.Context, orgID, email string) (*tenant.Invitation, error) {
	var inv *tenant.Invitation
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetInvitationByEmail(ctx, gen.GetInvitationByEmailParams{OrgID: oid, Email: email})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		inv = invFromRow(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *InvitationRepo) ListPending(ctx context.Context, orgID string) ([]tenant.Invitation, error) {
	var out []tenant.Invitation
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListPendingInvitations(ctx, oid)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, *invFromRow(row))
		}
		return nil
	})
	return out, err
}

// ResolveByToken calls the SECURITY DEFINER function on the raw pool — the
// sanctioned cross-tenant read (ADR-014). Hand-written (sqlc can't infer the
// function's RETURNS TABLE columns).
func (r *InvitationRepo) ResolveByToken(ctx context.Context, tokenHash []byte) (*tenant.Invitation, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, org_id, email, role, invited_by, expires_at, accepted_at, revoked_at
		 FROM app_invitation_by_token($1)`, tokenHash)
	var (
		id, orgID, invitedBy uuid.UUID
		email, role          string
		expires              time.Time
		accepted, revoked    *time.Time
	)
	if err := row.Scan(&id, &orgID, &email, &role, &invitedBy, &expires, &accepted, &revoked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("resolve invite by token: %w", err)
	}
	return &tenant.Invitation{
		ID: id.String(), OrgID: orgID.String(), Email: email, Role: tenant.OrgRole(role),
		TokenHash: tokenHash, InvitedBy: invitedBy.String(), ExpiresAt: expires,
		AcceptedAt: accepted, RevokedAt: revoked,
	}, nil
}

func (r *InvitationRepo) Revoke(ctx context.Context, orgID, id string) error {
	invID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("invite revoke: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.RevokeInvitation(ctx, gen.RevokeInvitationParams{OrgID: oid, ID: invID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *InvitationRepo) UpdateToken(ctx context.Context, orgID, id string, tokenHash []byte, expiresAt time.Time) error {
	invID, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("invite update token: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateInvitationToken(ctx, gen.UpdateInvitationTokenParams{
			TokenHash: tokenHash, ExpiresAt: expiresAt, OrgID: oid, ID: invID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
