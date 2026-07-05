// This file is the organizations / members / invitations surface
// (docs/08-API-SPEC.md §4). Org-root routes (create, list, accept) run under
// auth only; org-scoped routes run under TenantGuard (resolve + Casbin gate),
// so handlers here can trust the resolved TenantContext.
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/tenantuc"
)

// OrgHandlers serves the organization surface.
type OrgHandlers struct {
	svc    *tenantuc.Service
	logger *slog.Logger
}

// NewOrgHandlers builds OrgHandlers.
func NewOrgHandlers(svc *tenantuc.Service, logger *slog.Logger) *OrgHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &OrgHandlers{svc: svc, logger: logger}
}

// ---- DTOs -----------------------------------------------------------------

type orgResp struct {
	ID        string     `json:"id"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	LogoKey   string     `json:"logo_key,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func toOrgResp(o *tenant.Organization) orgResp {
	return orgResp{
		ID: o.ID, Slug: o.Slug, Name: o.Name, LogoKey: o.LogoKey,
		DeletedAt: o.DeletedAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

type membershipResp struct {
	OrgID     string    `json:"org_id"`
	Role      string    `json:"role"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type memberResp struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarKey string    `json:"avatar_key,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type invitationResp struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invited_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func toInvitationResp(i *tenant.Invitation) invitationResp {
	return invitationResp{
		ID: i.ID, Email: i.Email, Role: string(i.Role), InvitedBy: i.InvitedBy,
		ExpiresAt: i.ExpiresAt, CreatedAt: i.CreatedAt,
	}
}

// ---- Org root (auth only) -------------------------------------------------

type createOrgReq struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CreateOrg creates an org owned by the caller.
func (h *OrgHandlers) CreateOrg(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	var req createOrgReq
	if !decodeJSON(w, r, &req) {
		return
	}
	org, err := h.svc.CreateOrg(r.Context(), p.UserID, req.Name, req.Slug)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toOrgResp(org))
}

// ListMyOrgs returns the caller's orgs.
func (h *OrgHandlers) ListMyOrgs(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	orgs, err := h.svc.ListMyOrgs(r.Context(), p.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	items := make([]membershipResp, 0, len(orgs))
	for _, o := range orgs {
		items = append(items, membershipResp{
			OrgID: o.OrgID, Role: string(o.Role), Slug: o.Slug, Name: o.Name, CreatedAt: o.CreatedAt,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type acceptInviteReq struct {
	Token string `json:"token"`
}

// AcceptInvitation binds a pending invitation to the caller.
func (h *OrgHandlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	var req acceptInviteReq
	if !decodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.AcceptInvitation(r.Context(), p.UserID, req.Token)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"org_id": out.OrgID, "role": string(out.Role)})
}

// ---- Org scoped (TenantGuard) ---------------------------------------------

// GetOrg returns the resolved org (any member).
func (h *OrgHandlers) GetOrg(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	org, err := h.svc.GetOrg(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toOrgResp(org))
}

type updateOrgReq struct {
	Name    *string `json:"name"`
	LogoKey *string `json:"logo_key"`
	Slug    *string `json:"slug"`
}

// UpdateOrg edits name/logo (ADMIN+, via route gate) or slug (OWNER only, a
// fine gate checked here since the coarse gate already admitted ADMIN+).
func (h *OrgHandlers) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	var req updateOrgReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// Slug changes require OWNER (docs/05 §2, docs/08 §4).
	if req.Slug != nil {
		if tc.Role != tenant.RoleOwner {
			response.Error(w, domain.ErrForbidden)
			return
		}
		if _, err := h.svc.ChangeSlug(r.Context(), tc.OrgID, *req.Slug); err != nil {
			response.Error(w, err)
			return
		}
	}
	if req.Name != nil {
		org, err := h.svc.UpdateProfile(r.Context(), tc.OrgID, *req.Name, req.LogoKey)
		if err != nil {
			response.Error(w, err)
			return
		}
		response.JSON(w, http.StatusOK, toOrgResp(org))
		return
	}
	// Slug-only (or no-op) update: return the current org.
	org, err := h.svc.GetOrg(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toOrgResp(org))
}

// DeleteOrg soft-deletes the org (OWNER).
func (h *OrgHandlers) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	if err := h.svc.SoftDelete(r.Context(), tc.OrgID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreOrg restores a soft-deleted org within grace (OWNER).
func (h *OrgHandlers) RestoreOrg(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	if err := h.svc.Restore(r.Context(), tc.OrgID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type transferReq struct {
	UserID string `json:"user_id"`
}

// TransferOwnership moves OWNER to another member (OWNER).
func (h *OrgHandlers) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	var req transferReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.TransferOwnership(r.Context(), tc.OrgID, tc.UserID, req.UserID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Members --------------------------------------------------------------

// ListMembers returns the org members (any member).
func (h *OrgHandlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	members, err := h.svc.ListMembers(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	items := make([]memberResp, 0, len(members))
	for _, m := range members {
		items = append(items, memberResp{
			UserID: m.UserID, Email: m.Email, Name: m.Name, AvatarKey: m.AvatarKey,
			Role: string(m.Role), CreatedAt: m.CreatedAt,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type changeRoleReq struct {
	Role string `json:"role"`
}

// ChangeMemberRole updates a member's role (ADMIN+).
func (h *OrgHandlers) ChangeMemberRole(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	userID := chi.URLParam(r, "userId")
	var req changeRoleReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ChangeMemberRole(r.Context(), tc.OrgID, tc.Role, userID, tenant.OrgRole(req.Role)); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember removes a member (ADMIN+).
func (h *OrgHandlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	userID := chi.URLParam(r, "userId")
	if err := h.svc.RemoveMember(r.Context(), tc.OrgID, tc.Role, userID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Leave removes the caller's own membership (any member).
func (h *OrgHandlers) Leave(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	if err := h.svc.Leave(r.Context(), tc.OrgID, tc.UserID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Invitations ----------------------------------------------------------

// ListInvitations returns pending invitations (ADMIN+).
func (h *OrgHandlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	invs, err := h.svc.ListInvitations(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	items := make([]invitationResp, 0, len(invs))
	for i := range invs {
		items = append(items, toInvitationResp(&invs[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type createInviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// CreateInvitation invites an email (ADMIN+).
func (h *OrgHandlers) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	var req createInviteReq
	if !decodeJSON(w, r, &req) {
		return
	}
	inv, err := h.svc.CreateInvitation(r.Context(), tc.OrgID, tc.UserID, req.Email, tenant.OrgRole(req.Role))
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, toInvitationResp(inv))
}

// RevokeInvitation revokes a pending invitation (ADMIN+).
func (h *OrgHandlers) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.RevokeInvitation(r.Context(), tc.OrgID, id); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResendInvitation rotates + re-sends a pending invitation (ADMIN+).
func (h *OrgHandlers) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	tc, _ := mw.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.ResendInvitation(r.Context(), tc.OrgID, id); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
