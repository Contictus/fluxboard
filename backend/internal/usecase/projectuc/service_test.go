package projectuc

import (
	"context"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// Fakes embed the domain interface so only the methods a test exercises need
// implementing; any other call panics (nil method), keeping the fakes honest
// about what the code under test touches (mirrors taskuc/service_test.go).

type fakeProjects struct {
	project.ProjectRepository
	p          *project.Project
	created    *project.Project
	createErr  error
}

func (f *fakeProjects) Get(_ context.Context, _, _ string) (*project.Project, error) {
	// CreateProject reloads via Get after inserting; return the just-created row
	// when the test didn't seed an explicit fixture.
	if f.p != nil {
		return f.p, nil
	}
	if f.created != nil {
		return f.created, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeProjects) Create(_ context.Context, _ string, p *project.Project) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = p
	return nil
}

type fakeMembers struct {
	project.ProjectMemberRepository
	role  *project.ProjectRole
	added int
}

func (f *fakeMembers) Get(_ context.Context, _, _, _ string) (*project.ProjectMember, error) {
	if f.role == nil {
		return nil, domain.ErrNotFound
	}
	return &project.ProjectMember{Role: *f.role}, nil
}

func (f *fakeMembers) Add(_ context.Context, _, _, _ string, _ project.ProjectRole) error {
	f.added++
	return nil
}

type fakeBoards struct {
	project.BoardRepository
	created int
}

func (f *fakeBoards) Create(_ context.Context, _ string, _ *project.Board) error {
	f.created++
	return nil
}

type fakeColumns struct {
	project.ColumnRepository
	created int
}

func (f *fakeColumns) Create(_ context.Context, _ string, _ *project.Column) error {
	f.created++
	return nil
}

func TestCreateProject_Validation(t *testing.T) {
	ctx := context.Background()
	s := New(Deps{Projects: &fakeProjects{}, Members: &fakeMembers{}, Boards: &fakeBoards{}, Columns: &fakeColumns{}})

	cases := []struct {
		name string
		in   CreateProjectInput
	}{
		{"bad key", CreateProjectInput{Key: "1", Name: "X"}},
		{"empty name", CreateProjectInput{Key: "WEB", Name: "   "}},
		{"bad visibility", CreateProjectInput{Key: "WEB", Name: "X", Visibility: project.Visibility("nope")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.CreateProject(ctx, "o", "u", c.in); err != domain.ErrValidation {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}

func TestCreateProject_HappyPath(t *testing.T) {
	ctx := context.Background()
	projects := &fakeProjects{}
	members := &fakeMembers{}
	boards := &fakeBoards{}
	columns := &fakeColumns{}
	s := New(Deps{Projects: projects, Members: members, Boards: boards, Columns: columns})

	// lower-case key + surrounding whitespace should normalize; blank visibility defaults to org.
	got, err := s.CreateProject(ctx, "o", "u", CreateProjectInput{Key: " web ", Name: " Website "})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if got.Key != "WEB" {
		t.Errorf("key = %q, want WEB (upper-cased)", got.Key)
	}
	if got.Name != "Website" {
		t.Errorf("name = %q, want trimmed Website", got.Name)
	}
	if got.Visibility != project.VisibilityOrg {
		t.Errorf("visibility = %q, want default org", got.Visibility)
	}
	if got.CreatedBy != "u" {
		t.Errorf("created_by = %q, want u", got.CreatedBy)
	}
	if members.added != 1 {
		t.Errorf("member Add called %d times, want 1 (creator as LEAD)", members.added)
	}
	if boards.created != 1 {
		t.Errorf("board created %d times, want 1", boards.created)
	}
	if columns.created != len(project.DefaultColumns) {
		t.Errorf("columns created %d, want %d seeded", columns.created, len(project.DefaultColumns))
	}
}

func TestCreateProject_ConflictPropagates(t *testing.T) {
	ctx := context.Background()
	s := New(Deps{
		Projects: &fakeProjects{createErr: domain.ErrConflict},
		Members:  &fakeMembers{}, Boards: &fakeBoards{}, Columns: &fakeColumns{},
	})
	if _, err := s.CreateProject(ctx, "o", "u", CreateProjectInput{Key: "WEB", Name: "X"}); err != domain.ErrConflict {
		t.Fatalf("err = %v, want ErrConflict on duplicate key", err)
	}
}

// access mirrors the project-role authorization matrix (FR-PROJ-002/003).
func TestAccess(t *testing.T) {
	ctx := context.Background()
	orgProj := &project.Project{ID: "p1", Visibility: project.VisibilityOrg}
	privProj := &project.Project{ID: "p1", Visibility: project.VisibilityPrivate}

	svc := func(p *project.Project, r *project.ProjectRole) *Service {
		return New(Deps{Projects: &fakeProjects{p: p}, Members: &fakeMembers{role: r}})
	}
	role := func(r project.ProjectRole) *project.ProjectRole { return &r }

	// Org ADMIN+ is an implicit LEAD regardless of membership.
	if _, got, err := svc(orgProj, nil).access(ctx, "o", "p1", "u", tenant.RoleAdmin, project.RoleLead); err != nil || got != project.RoleLead {
		t.Fatalf("org admin: got %q err %v, want LEAD nil", got, err)
	}
	// Non-member of a private project is opaque (ErrNotFound).
	if _, _, err := svc(privProj, nil).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleViewer); err != domain.ErrNotFound {
		t.Fatalf("private non-member: err %v, want ErrNotFound", err)
	}
	// A CONTRIBUTOR member cannot perform LEAD-only actions.
	if _, _, err := svc(privProj, role(project.RoleContributor)).access(ctx, "o", "p1", "u", tenant.RoleMember, project.RoleLead); err != domain.ErrForbidden {
		t.Fatalf("contributor wanting lead: err %v, want ErrForbidden", err)
	}
}
