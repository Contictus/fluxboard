// This file is the account self-service surface (docs/08-API-SPEC.md §3): the
// caller's own profile, avatar, and account deletion. All routes run under auth;
// there is no org context (the endpoints are org-independent).
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/useruc"
)

// UserHandlers serves the /me surface.
type UserHandlers struct {
	svc    *useruc.Service
	logger *slog.Logger
}

// NewUserHandlers builds UserHandlers.
func NewUserHandlers(svc *useruc.Service, logger *slog.Logger) *UserHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserHandlers{svc: svc, logger: logger}
}

type profileResp struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	PlatformRole  string    `json:"platform_role"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

func toProfileResp(p *useruc.Profile) profileResp {
	return profileResp{
		ID: p.ID, Email: p.Email, Name: p.Name, AvatarURL: p.AvatarURL,
		EmailVerified: p.EmailVerified, PlatformRole: p.PlatformRole,
		TOTPEnabled: p.TOTPEnabled, CreatedAt: p.CreatedAt,
	}
}

// Me returns the caller's profile (GET /me).
func (h *UserHandlers) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	prof, err := h.svc.Get(r.Context(), p.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toProfileResp(prof))
}

type updateMeReq struct {
	Name string `json:"name"`
}

// UpdateMe changes the caller's display name (PATCH /me) and returns the fresh profile.
func (h *UserHandlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	var req updateMeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.UpdateName(r.Context(), p.UserID, req.Name); err != nil {
		response.Error(w, err)
		return
	}
	prof, err := h.svc.Get(r.Context(), p.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, toProfileResp(prof))
}

type avatarUploadReq struct {
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type avatarUploadResp struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

// AvatarUploadURL issues a presigned PUT for the caller's avatar (POST /me/avatar/upload-url).
func (h *UserHandlers) AvatarUploadURL(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	var req avatarUploadReq
	if !decodeJSON(w, r, &req) {
		return
	}
	key, url, err := h.svc.RequestAvatarUpload(r.Context(), p.UserID, req.ContentType, req.Size)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, avatarUploadResp{Key: key, URL: url})
}

type avatarConfirmReq struct {
	Key string `json:"key"`
}

// AvatarConfirm records the uploaded object as the caller's avatar (POST /me/avatar/confirm).
func (h *UserHandlers) AvatarConfirm(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	var req avatarConfirmReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ConfirmAvatar(r.Context(), p.UserID, req.Key); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteMe hard-deletes the caller's account (DELETE /me). When the caller is the
// sole owner of one or more orgs it returns 409 with the blocking list so the UI
// can prompt an ownership transfer first (docs/08 §3).
func (h *UserHandlers) DeleteMe(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	err := h.svc.Delete(r.Context(), p.UserID)
	if err != nil {
		var soleErr *useruc.SoleOwnerError
		if errors.As(err, &soleErr) {
			response.JSON(w, http.StatusConflict, response.Envelope{Error: response.ErrorBody{
				Code:      "conflict",
				Message:   "account is the sole owner of one or more organizations",
				Details:   map[string]any{"sole_owner_of": soleErr.Orgs},
				RequestID: w.Header().Get("X-Request-ID"),
			}})
			return
		}
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
