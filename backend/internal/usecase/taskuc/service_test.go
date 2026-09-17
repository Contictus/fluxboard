package taskuc

import (
	"context"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// fakeProjects / fakeMembers embed the domain interfaces so only the methods the
// tests exercise need implementing; any other call panics (nil method), which
// keeps the fakes honest about what the code under test touches.
type fakeProjects struct {
	project.ProjectRepository
	p *project.Project
}

func (f fakeProjects) Get(_ context.Context, _, _ string) (*project.Project, error) {
	if f.p == nil {
		return nil, domain.ErrNotFound
	}
	return f.p, nil
}

type fakeMembers struct {
	project.ProjectMemberRepository
	role *project.ProjectRole
}

func (f fakeMembers) Get(_ context.Context, _, _, _ string) (*project.ProjectMember, error) {
	if f.role == nil {
		return nil, domain.ErrNotFound
	}
	return &project.ProjectMember{Role: *f.role}, nil
}

func svc(p *project.Project, role *project.ProjectRole) *Service {
	return New(Deps{Projects: fakeProjects{p: p}, Members: fakeMembers{role: role}})
}

func role(r project.ProjectRole) *project.ProjectRole { return &r }

func TestAccess(t *testing.T) {
	orgProj := &project.Project{ID: "p1", Visibility: project.VisibilityOrg}
	privProj := &project.Project{ID: "p1", Visibility: project.VisibilityPrivate}
	ctx := context.Background()

	// Org ADMIN+ is implicit LEAD regardless of membership.
	got, err := svc(orgProj, nil).access(ctx, "o", "p1", "u", tenant.RoleAdmin, project.RoleLead)
	if err != nil || got != project.RoleLead {
		t.Fatalf("org admin: got %q err %v, want LEAD nil", got, err)
	}

	// Non-member of an org-visible project is a VIEWER (MEMBER+)…
	if got, err := svc(orgProj, nil).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleViewer); err != nil || got != project.RoleViewer {
		t.Fatalf("org-visible member: got %q err %v, want VIEWER nil", got, err)
	}
	// …but wanting CONTRIBUTOR there is forbidden.
	if _, err := svc(orgProj, nil).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleContributor); err != domain.ErrForbidden {
		t.Fatalf("org-visible viewer wanting contributor: err %v, want ErrForbidden", err)
	}
	// A GUEST non-member cannot see an org-visible project at all.
	if _, err := svc(orgProj, nil).access(ctx, "o", "p1", "u", tenant.RoleGuest, project.RoleViewer); err != domain.ErrNotFound {
		t.Fatalf("guest non-member: err %v, want ErrNotFound", err)
	}
	// A non-member of a private project gets ErrNotFound (opaque).
	if _, err := svc(privProj, nil).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleViewer); err != domain.ErrNotFound {
		t.Fatalf("private non-member: err %v, want ErrNotFound", err)
	}
	// A CONTRIBUTOR member of a private project gets their role…
	if got, err := svc(privProj, role(project.RoleContributor)).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleContributor); err != nil || got != project.RoleContributor {
		t.Fatalf("private contributor: got %q err %v", got, err)
	}
	// …and cannot perform LEAD-only actions.
	if _, err := svc(privProj, role(project.RoleContributor)).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleLead); err != domain.ErrForbidden {
		t.Fatalf("private contributor wanting lead: err %v, want ErrForbidden", err)
	}
}

func TestDiffTask(t *testing.T) {
	due := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	alice := "alice"
	base := &project.Task{
		Title: "old", Description: "d", AssigneeID: nil,
		Priority: project.PriorityLow, DueDate: nil,
	}
	in := UpdateTaskInput{
		Title: "new", Description: "d", AssigneeID: &alice,
		Priority: project.PriorityHigh, StartDate: &start, DueDate: &due,
	}
	changes := diffTask(base, in)
	fields := map[string]bool{}
	for _, c := range changes {
		fields[c.field] = true
	}
	for _, want := range []string{"title", "assignee", "priority", "start_date", "due_date"} {
		if !fields[want] {
			t.Errorf("expected a %q change", want)
		}
	}
	if fields["description"] {
		t.Error("description did not change; should not be logged")
	}
	if len(changes) != 5 {
		t.Errorf("got %d changes, want 5", len(changes))
	}
}

func TestCheckDateRange(t *testing.T) {
	early := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	if err := checkDateRange(&early, &late); err != nil {
		t.Errorf("ordered pair: %v", err)
	}
	if err := checkDateRange(nil, &late); err != nil {
		t.Errorf("nil start: %v", err)
	}
	if err := checkDateRange(&early, nil); err != nil {
		t.Errorf("nil due: %v", err)
	}
	if err := checkDateRange(&late, &early); err != domain.ErrValidation {
		t.Errorf("start after due: err = %v, want ErrValidation", err)
	}
}
