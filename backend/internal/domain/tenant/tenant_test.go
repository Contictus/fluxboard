package tenant

import "testing"

func TestRoleOrdering(t *testing.T) {
	if !RoleOwner.AtLeast(RoleAdmin) || !RoleAdmin.AtLeast(RoleMember) || !RoleMember.AtLeast(RoleGuest) {
		t.Fatal("role hierarchy broken")
	}
	if RoleGuest.AtLeast(RoleMember) {
		t.Fatal("guest must not be >= member")
	}
	if RoleOrgRoleValid := OrgRole("BOGUS").Valid(); RoleOrgRoleValid {
		t.Fatal("unknown role must be invalid")
	}
}

func TestAssignable(t *testing.T) {
	if RoleOwner.Assignable() {
		t.Fatal("OWNER must not be directly assignable (transfer only)")
	}
	for _, r := range []OrgRole{RoleAdmin, RoleMember, RoleGuest} {
		if !r.Assignable() {
			t.Fatalf("%s should be assignable", r)
		}
	}
}
