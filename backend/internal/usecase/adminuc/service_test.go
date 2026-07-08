package adminuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
)

type fakeAdminRepo struct{ found bool }

func (f *fakeAdminRepo) ListTenants(context.Context, admin.TenantFilter) ([]admin.TenantSummary, error) {
	return nil, nil
}
func (f *fakeAdminRepo) GetTenant(_ context.Context, orgID string) (admin.TenantSummary, error) {
	if !f.found {
		return admin.TenantSummary{}, domain.ErrNotFound
	}
	return admin.TenantSummary{OrgID: orgID}, nil
}
func (f *fakeAdminRepo) WebhookEventsForCustomer(context.Context, string, int) ([]admin.WebhookEvent, error) {
	return nil, nil
}

type fakeMinter struct{ called bool }

func (f *fakeMinter) Mint(_, _, _ string) (string, time.Time, error) {
	f.called = true
	return "tok", time.Now().Add(time.Minute), nil
}

type fakeAudit struct{ entries []audit.Entry }

func (f *fakeAudit) Append(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

func TestStartImpersonationAuditsBothIdentities(t *testing.T) {
	aud := &fakeAudit{}
	svc := New(Deps{Tenants: &fakeAdminRepo{found: true}, Minter: &fakeMinter{}, Audit: aud})

	tok, _, err := svc.StartImpersonation(context.Background(), "admin1", "sid1", "orgT")
	if err != nil {
		t.Fatalf("impersonate: %v", err)
	}
	if tok != "tok" {
		t.Fatalf("token = %q, want tok", tok)
	}
	if len(aud.entries) != 1 {
		t.Fatalf("want exactly one audit row, got %d", len(aud.entries))
	}
	e := aud.entries[0]
	if e.Action != audit.ActionImpersonateStart {
		t.Fatalf("action = %q, want %q", e.Action, audit.ActionImpersonateStart)
	}
	if e.Severity != audit.SeveritySecurity {
		t.Fatalf("severity = %q, want security", e.Severity)
	}
	if e.OrgID != "orgT" {
		t.Fatalf("org = %q, want target orgT", e.OrgID)
	}
	// Dual identity: both the actor and the impersonator are the admin.
	if e.ActorUserID != "admin1" || e.ImpersonatorUserID != "admin1" {
		t.Fatalf("identities actor=%q imp=%q, want both admin1", e.ActorUserID, e.ImpersonatorUserID)
	}
}

func TestStartImpersonationRequiresMinter(t *testing.T) {
	svc := New(Deps{Tenants: &fakeAdminRepo{found: true}}) // no minter
	if _, _, err := svc.StartImpersonation(context.Background(), "a", "s", "orgT"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no minter: want ErrForbidden, got %v", err)
	}
}

func TestStartImpersonationUnknownOrg(t *testing.T) {
	minter := &fakeMinter{}
	svc := New(Deps{Tenants: &fakeAdminRepo{found: false}, Minter: minter, Audit: &fakeAudit{}})
	if _, _, err := svc.StartImpersonation(context.Background(), "a", "s", "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown org: want ErrNotFound, got %v", err)
	}
	if minter.called {
		t.Fatal("must not mint a token for a nonexistent org")
	}
}
