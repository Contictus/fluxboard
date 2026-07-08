// This file is the org settings API-key surface (FR-API-001). All routes are
// ADMIN-only (write:apikeys in the Casbin policy). The secret is revealed exactly
// once, in the Create response; thereafter only its prefix is ever returned.
package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/apikey"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/apikeyuc"
)

// APIKeyHandlers serves the org-scoped API-key management endpoints.
type APIKeyHandlers struct {
	svc *apikeyuc.Service
	log *slog.Logger
}

// NewAPIKeyHandlers builds APIKeyHandlers.
func NewAPIKeyHandlers(svc *apikeyuc.Service, log *slog.Logger) *APIKeyHandlers {
	if log == nil {
		log = slog.Default()
	}
	return &APIKeyHandlers{svc: svc, log: log}
}

type apiKeyResp struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toAPIKeyResp(k apikey.APIKey) apiKeyResp {
	scopes := make([]string, len(k.Scopes))
	for i, s := range k.Scopes {
		scopes[i] = string(s)
	}
	return apiKeyResp{
		ID: k.ID, Prefix: k.Prefix, Name: k.Name, Scopes: scopes,
		LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt, CreatedAt: k.CreatedAt,
	}
}

// List returns the org's API keys (secrets never included).
// @Summary  List API keys
// @Tags     api-keys
// @Security BearerAuth
// @Produce  json
// @Param    orgId  path  string  true  "Organization ID"
// @Success  200  {object}  map[string]interface{}
// @Router   /orgs/{orgId}/api-keys [get]
func (h *APIKeyHandlers) List(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	ks, err := h.svc.List(r.Context(), tc.OrgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]apiKeyResp, 0, len(ks))
	for _, k := range ks {
		out = append(out, toAPIKeyResp(k))
	}
	response.JSON(w, http.StatusOK, map[string]any{"api_keys": out})
}

type createAPIKeyReq struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// Create: POST /orgs/{orgId}/api-keys — returns the plaintext secret ONCE.
func (h *APIKeyHandlers) Create(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req createAPIKeyReq
	if !decodeJSON(w, r, &req) {
		return
	}
	scopes := make([]apikey.Scope, 0, len(req.Scopes))
	for _, s := range req.Scopes {
		scopes = append(scopes, apikey.Scope(s))
	}
	// A key minted via a session admin records that user; a key minting another key
	// (API-key principal) has no user id to attribute — store NULL.
	createdBy := tc.UserID
	if strings.HasPrefix(createdBy, "apikey:") {
		createdBy = ""
	}
	c, err := h.svc.Create(r.Context(), tc.OrgID, req.Name, createdBy, scopes)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, map[string]any{
		"api_key": toAPIKeyResp(c.Key),
		"secret":  c.Plaintext, // shown once; never retrievable again
	})
}

// Revoke: DELETE /orgs/{orgId}/api-keys/{id}
func (h *APIKeyHandlers) Revoke(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.Revoke(r.Context(), tc.OrgID, id, tc.UserID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
