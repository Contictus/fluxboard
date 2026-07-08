// This file is the org-scoped audit-log viewer + CSV export (FR-AUD-003). Both
// routes are ADMIN-only (read:audit). The usecase forces the org_id filter, the
// isolation backstop for a non-RLS table. CSV is bounded at audit.ExportCap (10k).
package handlers

import (
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/audituc"
)

// AuditHandlers serves the org audit viewer.
type AuditHandlers struct {
	svc *audituc.Service
	log *slog.Logger
}

// NewAuditHandlers builds AuditHandlers.
func NewAuditHandlers(svc *audituc.Service, log *slog.Logger) *AuditHandlers {
	if log == nil {
		log = slog.Default()
	}
	return &AuditHandlers{svc: svc, log: log}
}

// List: GET /orgs/{orgId}/audit?actor=&action=&severity=&since=&until=&limit=
func (h *AuditHandlers) List(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	entries, err := h.svc.ListForOrg(r.Context(), tc.OrgID, auditFilterFromQuery(r), false)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"entries": toAuditEntries(entries)})
}

// ExportCSV: GET /orgs/{orgId}/audit.csv — streams up to ExportCap rows.
func (h *AuditHandlers) ExportCSV(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	entries, err := h.svc.ListForOrg(r.Context(), tc.OrgID, auditFilterFromQuery(r), true)
	if err != nil {
		response.Error(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"created_at", "action", "severity", "actor_user_id",
		"impersonator_user_id", "target_type", "target_id", "ip", "metadata"})
	for _, e := range entries {
		meta := ""
		if len(e.Metadata) > 0 {
			if b, err := json.Marshal(e.Metadata); err == nil {
				meta = string(b)
			}
		}
		_ = cw.Write([]string{
			e.CreatedAt.UTC().Format(time.RFC3339),
			string(e.Action), string(e.Severity), e.ActorUserID, e.ImpersonatorUserID,
			e.TargetType, e.TargetID, e.IP, meta,
		})
	}
	cw.Flush()
}
