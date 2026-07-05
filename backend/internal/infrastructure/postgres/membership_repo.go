package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// MembershipRepo is the Postgres-backed tenant.MembershipRepository. memberships
// is [T] (RLS), so every method runs through TenantPool.WithTenant — the tenant
// GUC makes RLS the backstop under the org_id filter (invariant #1).
type MembershipRepo struct {
	tp *TenantPool
}

// NewMembershipRepo builds a MembershipRepo over the tenant pool.
func NewMembershipRepo(tp *TenantPool) *MembershipRepo { return &MembershipRepo{tp: tp} }

var _ tenant.MembershipRepository = (*MembershipRepo)(nil)

func (r *MembershipRepo) Get(ctx context.Context, orgID, userID string) (*tenant.Membership, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("membership get: %w", err)
	}
	var m *tenant.Membership
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetMembership(ctx, gen.GetMembershipParams{OrgID: oid, UserID: uid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		m = &tenant.Membership{
			OrgID: row.OrgID.String(), UserID: row.UserID.String(),
			Role: tenant.OrgRole(row.Role), CreatedAt: row.CreatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *MembershipRepo) List(ctx context.Context, orgID string, mq tenant.MemberQuery) ([]tenant.Member, error) {
	limit := mq.Limit
	if limit <= 0 {
		limit = 50
	}
	var rolePtr *string
	if mq.Role != nil {
		s := string(*mq.Role)
		rolePtr = &s
	}
	var qPtr *string
	if qv := mq.Q; qv != "" {
		qPtr = &qv
	}
	after := pgtype.Timestamptz{}
	afterUser := pgtype.UUID{}
	if !mq.AfterCreated.IsZero() {
		after = pgtype.Timestamptz{Time: mq.AfterCreated, Valid: true}
		if uid, err := parseUUID(mq.AfterUser); err == nil {
			afterUser = pgtype.UUID{Bytes: uid, Valid: true}
		}
	}

	var out []tenant.Member
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		rows, err := q.ListMembersFiltered(ctx, gen.ListMembersFilteredParams{
			OrgID: oid, Role: rolePtr, Q: qPtr,
			AfterCreated: after, AfterUser: afterUser, Lim: int32(limit),
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, tenant.Member{
				UserID: row.UserID.String(), Email: row.Email, Name: row.Name,
				AvatarKey: row.AvatarKey, Role: tenant.OrgRole(row.Role), CreatedAt: row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

func (r *MembershipRepo) UpdateRole(ctx context.Context, orgID, userID string, role tenant.OrgRole) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("membership update role: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateMemberRole(ctx, gen.UpdateMemberRoleParams{
			Role: string(role), OrgID: oid, UserID: uid,
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

func (r *MembershipRepo) Delete(ctx context.Context, orgID, userID string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("membership delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteMembership(ctx, gen.DeleteMembershipParams{OrgID: oid, UserID: uid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func (r *MembershipRepo) CountByRole(ctx context.Context, orgID string, role tenant.OrgRole) (int, error) {
	var n int
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		c, err := q.CountMembersByRole(ctx, gen.CountMembersByRoleParams{OrgID: oid, Role: string(role)})
		n = int(c)
		return err
	})
	return n, err
}

func (r *MembershipRepo) TransferOwnership(ctx context.Context, orgID, fromUserID, toUserID string) error {
	fromID, err := parseUUID(fromUserID)
	if err != nil {
		return fmt.Errorf("transfer ownership: from: %w", err)
	}
	toID, err := parseUUID(toUserID)
	if err != nil {
		return fmt.Errorf("transfer ownership: to: %w", err)
	}
	oid, _ := parseUUID(orgID)
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		// Promote target first; ErrNotFound if they are not a member.
		n, err := q.UpdateMemberRole(ctx, gen.UpdateMemberRoleParams{
			Role: string(tenant.RoleOwner), OrgID: oid, UserID: toID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		if _, err := q.UpdateMemberRole(ctx, gen.UpdateMemberRoleParams{
			Role: string(tenant.RoleAdmin), OrgID: oid, UserID: fromID,
		}); err != nil {
			return err
		}
		return nil
	})
}

func (r *MembershipRepo) AcceptInvitation(ctx context.Context, orgID, invitationID, userID string, role tenant.OrgRole) error {
	invID, err := parseUUID(invitationID)
	if err != nil {
		return fmt.Errorf("accept invite: id: %w", err)
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return fmt.Errorf("accept invite: user: %w", err)
	}
	oid, _ := parseUUID(orgID)
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		// Consume the invitation; 0 rows ⇒ already accepted/revoked/expired.
		n, err := q.MarkInvitationAccepted(ctx, gen.MarkInvitationAcceptedParams{OrgID: oid, ID: invID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflict
		}
		return q.CreateMembership(ctx, gen.CreateMembershipParams{
			OrgID: oid, UserID: uid, Role: string(role),
		})
	})
	if isUnique(err) {
		return domain.ErrConflict // already a member
	}
	return err
}
