package project

import (
	"testing"
	"time"
)

func TestProjectRoleAtLeast(t *testing.T) {
	cases := []struct {
		role, min ProjectRole
		want      bool
	}{
		{RoleLead, RoleContributor, true},
		{RoleContributor, RoleContributor, true},
		{RoleViewer, RoleContributor, false},
		{RoleViewer, RoleViewer, true},
		{RoleLead, RoleLead, true},
		{RoleContributor, RoleLead, false},
	}
	for _, c := range cases {
		if got := c.role.AtLeast(c.min); got != c.want {
			t.Errorf("%s.AtLeast(%s) = %v, want %v", c.role, c.min, got, c.want)
		}
	}
	if ProjectRole("BOGUS").Valid() {
		t.Error("bogus role reported valid")
	}
}

func TestValidKey(t *testing.T) {
	valid := []string{"PAY", "AB", "ABCDEF", "XY"}
	invalid := []string{"A", "ABCDEFG", "pay", "PA1", "PA-", "", "AB C"}
	for _, k := range valid {
		if !ValidKey(k) {
			t.Errorf("ValidKey(%q) = false, want true", k)
		}
	}
	for _, k := range invalid {
		if ValidKey(k) {
			t.Errorf("ValidKey(%q) = true, want false", k)
		}
	}
}

func TestVisibilityAndPriorityValid(t *testing.T) {
	if !VisibilityOrg.Valid() || !VisibilityPrivate.Valid() || Visibility("nope").Valid() {
		t.Error("visibility validity wrong")
	}
	for _, p := range []Priority{PriorityUrgent, PriorityHigh, PriorityMedium, PriorityLow, PriorityNone} {
		if !p.Valid() {
			t.Errorf("priority %q should be valid", p)
		}
	}
	if Priority("critical").Valid() {
		t.Error("bogus priority reported valid")
	}
}

func TestCommentEditable(t *testing.T) {
	created := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	c := &Comment{CreatedAt: created}
	if !c.Editable(created.Add(5 * time.Minute)) {
		t.Error("comment should be editable within the window")
	}
	if c.Editable(created.Add(CommentEditWindow + time.Second)) {
		t.Error("comment should not be editable after the window")
	}
	deleted := created.Add(time.Minute)
	c.DeletedAt = &deleted
	if c.Editable(created.Add(2 * time.Minute)) {
		t.Error("deleted comment must not be editable")
	}
}
