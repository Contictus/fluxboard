package audituc

import (
	"context"
	"errors"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

// fakeReader captures the filter the service passes down.
type fakeReader struct{ last audit.Filter }

func (f *fakeReader) List(_ context.Context, flt audit.Filter) ([]audit.Entry, error) {
	f.last = flt
	return nil, nil
}

func TestListForOrgForcesOrgAndDefaultsLimit(t *testing.T) {
	r := &fakeReader{}
	svc := New(Deps{Reader: r})
	// Caller passes a different OrgID and no limit: the service must overwrite the
	// org (isolation backstop) and apply the viewer default.
	if _, err := svc.ListForOrg(context.Background(), "org1", audit.Filter{OrgID: "attacker"}, false); err != nil {
		t.Fatalf("list: %v", err)
	}
	if r.last.OrgID != "org1" {
		t.Fatalf("OrgID = %q, want forced org1", r.last.OrgID)
	}
	if r.last.Limit != defaultViewerLimit {
		t.Fatalf("Limit = %d, want default %d", r.last.Limit, defaultViewerLimit)
	}
}

func TestListForOrgEmptyOrgRejected(t *testing.T) {
	svc := New(Deps{Reader: &fakeReader{}})
	if _, err := svc.ListForOrg(context.Background(), "", audit.Filter{}, false); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty org: want ErrValidation, got %v", err)
	}
}

func TestExportCappedAt10k(t *testing.T) {
	r := &fakeReader{}
	svc := New(Deps{Reader: r})
	// An oversized export request is clamped to ExportCap (FR-AUD-003).
	if _, err := svc.ListForOrg(context.Background(), "org1", audit.Filter{Limit: 999999}, true); err != nil {
		t.Fatalf("export: %v", err)
	}
	if r.last.Limit != audit.ExportCap {
		t.Fatalf("export Limit = %d, want ExportCap %d", r.last.Limit, audit.ExportCap)
	}
	// A zero export limit also becomes the cap (stream the full allowed window).
	_, _ = svc.ListForOrg(context.Background(), "org1", audit.Filter{Limit: 0}, true)
	if r.last.Limit != audit.ExportCap {
		t.Fatalf("zero export Limit = %d, want ExportCap %d", r.last.Limit, audit.ExportCap)
	}
}

func TestListGlobalKeepsEmptyOrgAndCaps(t *testing.T) {
	r := &fakeReader{}
	svc := New(Deps{Reader: r})
	if _, err := svc.ListGlobal(context.Background(), audit.Filter{OrgID: "ignored", Limit: 50000}, true); err != nil {
		t.Fatalf("global: %v", err)
	}
	if r.last.OrgID != "" {
		t.Fatalf("global OrgID = %q, want empty (all orgs)", r.last.OrgID)
	}
	if r.last.Limit != audit.ExportCap {
		t.Fatalf("global export Limit = %d, want ExportCap", r.last.Limit)
	}
}
