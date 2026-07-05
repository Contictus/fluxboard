package tenantuc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
)

// AcceptOutcome is what AcceptInvitation returns on success.
type AcceptOutcome struct {
	OrgID string
	Role  tenant.OrgRole
}

// CreateInvitation invites an email to the org with a role (ADMIN+ gate).
// Guards: role must be assignable; the address must not already be a member or
// hold a pending invite (docs/05 §5). An expired/revoked prior invite for the
// same address is rotated in place (the UNIQUE(org_id,email) row is reused).
//
// The plan seat-count / pending-invite entitlement 402 gate (docs/05 §5) is
// deferred to Phase 4 with the billing tables.
func (s *Service) CreateInvitation(ctx context.Context, orgID, inviterID, email string, role tenant.OrgRole) (*tenant.Invitation, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return nil, fmt.Errorf("%w: valid email required", domain.ErrValidation)
	}
	if !role.Assignable() {
		return nil, fmt.Errorf("%w: role must be ADMIN, MEMBER, or GUEST", domain.ErrValidation)
	}

	// Reject if the address already belongs to a member.
	if u, err := s.users.GetByEmail(ctx, email); err == nil {
		if _, err := s.members.Get(ctx, orgID, u.ID); err == nil {
			return nil, fmt.Errorf("%w: already a member", domain.ErrConflict)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	raw, err := token.New()
	if err != nil {
		return nil, err
	}
	hash := token.Hash(raw)
	expires := s.now().Add(inviteTTL)

	// Reuse an existing (non-pending) row for this address if present; else insert.
	if existing, err := s.invites.GetByEmail(ctx, orgID, email); err == nil {
		if existing.Pending(s.now()) {
			return nil, fmt.Errorf("%w: an invitation is already pending for this address", domain.ErrConflict)
		}
		if err := s.invites.UpdateToken(ctx, orgID, existing.ID, hash, expires); err != nil {
			return nil, err
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		inv := &tenant.Invitation{
			ID: newID(), OrgID: orgID, Email: email, Role: role,
			TokenHash: hash, InvitedBy: inviterID, ExpiresAt: expires,
		}
		if err := s.invites.Create(ctx, inv); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	s.sendInvite(ctx, orgID, email, raw)
	return s.invites.GetByEmail(ctx, orgID, email)
}

// ListInvitations returns the org's pending invitations (ADMIN+ gate).
func (s *Service) ListInvitations(ctx context.Context, orgID string) ([]tenant.Invitation, error) {
	return s.invites.ListPending(ctx, orgID)
}

// RevokeInvitation revokes a pending invitation (ADMIN+ gate).
func (s *Service) RevokeInvitation(ctx context.Context, orgID, id string) error {
	return s.invites.Revoke(ctx, orgID, id)
}

// ResendInvitation rotates the token and expiry of a pending invitation and
// re-sends the email (ADMIN+ gate).
func (s *Service) ResendInvitation(ctx context.Context, orgID, id string) error {
	inv, err := s.invites.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if inv.AcceptedAt != nil || inv.RevokedAt != nil {
		return fmt.Errorf("%w: invitation is no longer pending", domain.ErrConflict)
	}
	raw, err := token.New()
	if err != nil {
		return err
	}
	if err := s.invites.UpdateToken(ctx, orgID, id, token.Hash(raw), s.now().Add(inviteTTL)); err != nil {
		return err
	}
	s.sendInvite(ctx, orgID, inv.Email, raw)
	return nil
}

// AcceptInvitation binds a pending invitation to the logged-in user. Per
// docs/05 §5 an email mismatch is permitted (the membership binds to the
// authenticated account); expired/used/revoked tokens are rejected.
func (s *Service) AcceptInvitation(ctx context.Context, userID, rawToken string) (*AcceptOutcome, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, fmt.Errorf("%w: token required", domain.ErrValidation)
	}
	inv, err := s.invites.ResolveByToken(ctx, token.Hash(rawToken))
	if err != nil {
		return nil, err // ErrNotFound → 404
	}
	if !inv.Pending(s.now()) {
		return nil, fmt.Errorf("%w: invitation expired or already used", domain.ErrConflict)
	}
	if err := s.members.AcceptInvitation(ctx, inv.OrgID, inv.ID, userID, inv.Role); err != nil {
		return nil, err
	}
	_ = s.cache.Invalidate(ctx, inv.OrgID, userID)
	return &AcceptOutcome{OrgID: inv.OrgID, Role: inv.Role}, nil
}

// sendInvite fetches the org name and dispatches the invitation email. A mail
// failure is logged, not fatal — the invite row already exists and can be
// resent.
func (s *Service) sendInvite(ctx context.Context, orgID, email, rawToken string) {
	orgName := ""
	if org, err := s.orgs.GetByID(ctx, orgID); err == nil {
		orgName = org.Name
	}
	if err := s.mailer.SendInvitation(ctx, email, orgName, rawToken); err != nil {
		s.logger.Warn("send invitation email failed", "err", err, "org", orgID)
	}
}

// validEmail is a minimal sanity check; citext UNIQUE + real delivery enforce
// the rest (matches the auth usecase's posture).
func validEmail(e string) bool {
	at := strings.IndexByte(e, '@')
	return at > 0 && at < len(e)-1 && !strings.ContainsAny(e, " \t\n")
}
