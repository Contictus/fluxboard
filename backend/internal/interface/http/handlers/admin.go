// This file is the platform-admin HTTP surface (docs/build/PHASE-6-ADMIN-OBS.md §5,
// FR-ADM-002..006). It is mounted on a SEPARATE /admin router, NOT under the tenant
// middleware: the platform admin guard (platform_role=admin + 2FA) is the only gate.
// Cross-tenant reads run on the owner pool inside adminuc; org isolation does not
// apply here by design.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/adminuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/audituc"
)

// JobsInspector summarizes the Asynq queues for /admin/jobs (FR-ADM-005). Nil ⇒ the
// endpoint reports it is unavailable. Implemented by an asynq.Inspector adapter.
type JobsInspector interface {
	Summary(ctx context.Context) (map[string]any, error)
}

// AdminHandlers serves the platform-admin surface.
type AdminHandlers struct {
	admin *adminuc.Service
	audit *audituc.Service
	jobs  JobsInspector
	log   *slog.Logger
}

// NewAdminHandlers builds AdminHandlers. jobs may be nil.
func NewAdminHandlers(a *adminuc.Service, au *audituc.Service, jobs JobsInspector, log *slog.Logger) *AdminHandlers {
	if log == nil {
		log = slog.Default()
	}
	return &AdminHandlers{admin: a, audit: au, jobs: jobs, log: log}
}

// ---- Tenants ---------------------------------------------------------------

// ListTenants: GET /admin/tenants?search=&plan=&status=&limit=&offset=
func (h *AdminHandlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := admin.TenantFilter{
		Search: q.Get("search"),
		Plan:   q.Get("plan"),
		Status: q.Get("status"),
		Limit:  atoiDefault(q.Get("limit"), 0),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	ts, err := h.admin.ListTenants(r.Context(), f)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]tenantSummaryResp, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTenantSummary(t))
	}
	response.JSON(w, http.StatusOK, map[string]any{"tenants": out})
}

// GetTenant: GET /admin/tenants/{orgId}
func (h *AdminHandlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	d, err := h.admin.GetTenantDetail(r.Context(), orgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	resp := map[string]any{
		"tenant":    toTenantSummary(d.Summary),
		"overrides": d.Overrides,
		"flags":     d.Flags,
		"webhooks":  d.Webhooks,
	}
	if d.Subscription != nil {
		resp["subscription"] = d.Subscription
	}
	resp["invoices"] = d.Invoices
	response.JSON(w, http.StatusOK, resp)
}

// ---- Impersonation ---------------------------------------------------------

// Impersonate: POST /admin/tenants/{orgId}/impersonate → one-time read-only token.
func (h *AdminHandlers) Impersonate(w http.ResponseWriter, r *http.Request) {
	p, ok := mw.PrincipalFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	orgID := chi.URLParam(r, "orgId")
	token, exp, err := h.admin.StartImpersonation(r.Context(), p.UserID, p.SID, orgID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": exp})
}

// ---- Webhook retry ---------------------------------------------------------

// RetryWebhook: POST /admin/tenants/{orgId}/webhooks/{eventId}/retry
func (h *AdminHandlers) RetryWebhook(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	orgID := chi.URLParam(r, "orgId")
	eventID := chi.URLParam(r, "eventId")
	if err := h.admin.RetryWebhook(r.Context(), orgID, eventID, p.UserID); err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusAccepted, map[string]any{"enqueued": eventID})
}

// ---- Feature flags + overrides --------------------------------------------

type setFlagReq struct {
	Flag    string `json:"flag"`
	Enabled bool   `json:"enabled"`
}

// SetFlag: PUT /admin/tenants/{orgId}/flags
func (h *AdminHandlers) SetFlag(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	orgID := chi.URLParam(r, "orgId")
	var req setFlagReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.admin.SetFlag(r.Context(), orgID, req.Flag, req.Enabled, p.UserID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setOverrideReq struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

// SetOverride: PUT /admin/tenants/{orgId}/overrides
func (h *AdminHandlers) SetOverride(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	orgID := chi.URLParam(r, "orgId")
	var req setOverrideReq
	if !decodeJSON(w, r, &req) {
		return
	}
	err := h.admin.SetOverride(r.Context(), admin.EntitlementOverride{
		OrgID: orgID, Key: req.Key, Value: req.Value, Note: req.Note, CreatedBy: p.UserID,
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteOverride: DELETE /admin/tenants/{orgId}/overrides/{key}
func (h *AdminHandlers) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	orgID := chi.URLParam(r, "orgId")
	key := chi.URLParam(r, "key")
	if err := h.admin.DeleteOverride(r.Context(), orgID, key, p.UserID); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Global audit + jobs ---------------------------------------------------

// GlobalAudit: GET /admin/audit?actor=&action=&severity=&since=&until=&limit=
func (h *AdminHandlers) GlobalAudit(w http.ResponseWriter, r *http.Request) {
	f := auditFilterFromQuery(r)
	entries, err := h.audit.ListGlobal(r.Context(), f, false)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"entries": toAuditEntries(entries)})
}

// Jobs: GET /admin/jobs — Asynq queue summary.
func (h *AdminHandlers) Jobs(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		response.JSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	sum, err := h.jobs.Summary(r.Context())
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, sum)
}

// ---- DTOs + helpers --------------------------------------------------------

type tenantSummaryResp struct {
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Plan        string    `json:"plan"`
	Status      string    `json:"status"`
	MemberCount int       `json:"member_count"`
	MRR         int64     `json:"mrr"`
	CreatedAt   time.Time `json:"created_at"`
}

func toTenantSummary(t admin.TenantSummary) tenantSummaryResp {
	return tenantSummaryResp{
		OrgID: t.OrgID, Name: t.Name, Slug: t.Slug, Plan: t.PlanCode, Status: t.Status,
		MemberCount: t.MemberCount, MRR: t.MRR, CreatedAt: t.CreatedAt,
	}
}

type auditEntryResp struct {
	OrgID              string         `json:"org_id,omitempty"`
	ActorUserID        string         `json:"actor_user_id,omitempty"`
	ImpersonatorUserID string         `json:"impersonator_user_id,omitempty"`
	Action             string         `json:"action"`
	TargetType         string         `json:"target_type,omitempty"`
	TargetID           string         `json:"target_id,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	IP                 string         `json:"ip,omitempty"`
	Severity           string         `json:"severity"`
	CreatedAt          time.Time      `json:"created_at"`
}

func toAuditEntries(es []audit.Entry) []auditEntryResp {
	out := make([]auditEntryResp, 0, len(es))
	for _, e := range es {
		out = append(out, auditEntryResp{
			OrgID: e.OrgID, ActorUserID: e.ActorUserID, ImpersonatorUserID: e.ImpersonatorUserID,
			Action: string(e.Action), TargetType: e.TargetType, TargetID: e.TargetID,
			Metadata: e.Metadata, IP: e.IP, Severity: string(e.Severity), CreatedAt: e.CreatedAt,
		})
	}
	return out
}

// auditFilterFromQuery builds an audit.Filter from the common query params (shared
// by the global and org viewers). OrgID is set by the caller, not from the query.
func auditFilterFromQuery(r *http.Request) audit.Filter {
	q := r.URL.Query()
	f := audit.Filter{
		Actor:    q.Get("actor"),
		Action:   audit.Action(q.Get("action")),
		Severity: audit.Severity(q.Get("severity")),
		Limit:    atoiDefault(q.Get("limit"), 0),
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t.UTC()
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = t.UTC()
		}
	}
	return f
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
