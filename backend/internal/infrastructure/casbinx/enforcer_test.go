package casbinx

import (
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

func mustEnforcer(t *testing.T) *Enforcer {
	t.Helper()
	e, err := New()
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	return e
}

// TestPermissionMatrix pins the docs/05 §2 matrix through the enforcer,
// including the OWNER>ADMIN>MEMBER>GUEST inheritance.
func TestPermissionMatrix(t *testing.T) {
	e := mustEnforcer(t)
	cases := []struct {
		role tenant.OrgRole
		obj  string
		act  string
		want bool
	}{
		// OWNER: everything, including ownership-only actions.
		{tenant.RoleOwner, tenant.ObjOwnership, tenant.ActWrite, true},
		{tenant.RoleOwner, tenant.ObjOrg, tenant.ActWrite, true},
		{tenant.RoleOwner, tenant.ObjMembers, tenant.ActWrite, true},
		{tenant.RoleOwner, tenant.ObjProjects, tenant.ActWrite, true},
		{tenant.RoleOwner, tenant.ObjOrg, tenant.ActRead, true},

		// ADMIN: org/member/invite/billing/apikeys/audit, but NOT ownership.
		{tenant.RoleAdmin, tenant.ObjOwnership, tenant.ActWrite, false},
		{tenant.RoleAdmin, tenant.ObjOrg, tenant.ActWrite, true},
		{tenant.RoleAdmin, tenant.ObjMembers, tenant.ActWrite, true},
		{tenant.RoleAdmin, tenant.ObjInvitations, tenant.ActWrite, true},
		{tenant.RoleAdmin, tenant.ObjBilling, tenant.ActRead, true},
		{tenant.RoleAdmin, tenant.ObjBilling, tenant.ActWrite, true},
		{tenant.RoleAdmin, tenant.ObjAudit, tenant.ActRead, true},
		{tenant.RoleAdmin, tenant.ObjProjects, tenant.ActWrite, true},
		{tenant.RoleAdmin, tenant.ObjAutomations, tenant.ActWrite, true},

		// MEMBER: create projects + manage labels + read org; nothing admin.
		{tenant.RoleMember, tenant.ObjProjects, tenant.ActWrite, true},
		{tenant.RoleMember, tenant.ObjLabels, tenant.ActWrite, true},
		{tenant.RoleMember, tenant.ObjOrg, tenant.ActRead, true},
		{tenant.RoleMember, tenant.ObjOrg, tenant.ActWrite, false},
		{tenant.RoleMember, tenant.ObjMembers, tenant.ActWrite, false},
		{tenant.RoleMember, tenant.ObjBilling, tenant.ActRead, false},
		{tenant.RoleMember, tenant.ObjBilling, tenant.ActWrite, false},
		{tenant.RoleMember, tenant.ObjOwnership, tenant.ActWrite, false},
		{tenant.RoleMember, tenant.ObjAutomations, tenant.ActWrite, false},

		// GUEST: read org only.
		{tenant.RoleGuest, tenant.ObjOrg, tenant.ActRead, true},
		{tenant.RoleGuest, tenant.ObjProjects, tenant.ActWrite, false},
		{tenant.RoleGuest, tenant.ObjLabels, tenant.ActWrite, false},
		{tenant.RoleGuest, tenant.ObjMembers, tenant.ActWrite, false},
	}
	for _, c := range cases {
		got, err := e.Allowed(c.role, c.obj, c.act)
		if err != nil {
			t.Fatalf("enforce %s %s %s: %v", c.role, c.obj, c.act, err)
		}
		if got != c.want {
			t.Errorf("Allowed(%s, %s, %s) = %v, want %v", c.role, c.obj, c.act, got, c.want)
		}
	}
}

func TestUnknownRoleDenied(t *testing.T) {
	e := mustEnforcer(t)
	got, err := e.Allowed(tenant.OrgRole("BOGUS"), tenant.ObjOrg, tenant.ActRead)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("unknown role must be denied")
	}
}
