// Package audituc serves the audit-log viewers: the org-scoped viewer with CSV
// export (FR-AUD-003) and the platform-admin global viewer (FR-ADM-005). Read-only;
// domain-only deps. audit_log is not tenant-scoped (RLS), so org isolation is an
// explicit OrgID on the filter — the org path MUST set it (enforced here).
package audituc

import (
	"context"
	"fmt"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

// defaultViewerLimit bounds a viewer page when the caller passes no limit.
const defaultViewerLimit = 200

// Service implements the audit viewers.
type Service struct {
	reader audit.Reader
}

// Deps wires the service.
type Deps struct {
	Reader audit.Reader
}

// New builds the service.
func New(d Deps) *Service { return &Service{reader: d.Reader} }

// ListForOrg returns an org's audit entries per the filter. It forces f.OrgID to
// orgID (the isolation backstop for a non-RLS table) and bounds the limit. Used by
// both the viewer (default limit) and CSV export (up to ExportCap).
func (s *Service) ListForOrg(ctx context.Context, orgID string, f audit.Filter, export bool) ([]audit.Entry, error) {
	if orgID == "" {
		return nil, fmt.Errorf("%w: org required", domain.ErrValidation)
	}
	f.OrgID = orgID
	return s.reader.List(ctx, capFilter(f, export))
}

// ListGlobal returns entries across all orgs (platform-admin only, FR-ADM-005). The
// empty OrgID is intentional here and must only be reachable behind the admin guard.
func (s *Service) ListGlobal(ctx context.Context, f audit.Filter, export bool) ([]audit.Entry, error) {
	f.OrgID = "" // global
	return s.reader.List(ctx, capFilter(f, export))
}

// cap normalizes the row limit: export allows up to ExportCap; a viewer page
// defaults to defaultViewerLimit; neither exceeds ExportCap.
func capFilter(f audit.Filter, export bool) audit.Filter {
	switch {
	case export:
		if f.Limit <= 0 || f.Limit > audit.ExportCap {
			f.Limit = audit.ExportCap
		}
	default:
		if f.Limit <= 0 {
			f.Limit = defaultViewerLimit
		}
		if f.Limit > audit.ExportCap {
			f.Limit = audit.ExportCap
		}
	}
	return f
}
