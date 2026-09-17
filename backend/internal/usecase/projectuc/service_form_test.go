package projectuc

import (
	"context"
	"strings"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
)

type fakeForms struct {
	project.FormRepository
	forms   map[string]*project.ProjectForm
	byToken map[string]*project.ProjectForm
	created *project.ProjectForm
}

func newFakeForms(fs ...*project.ProjectForm) *fakeForms {
	f := &fakeForms{forms: make(map[string]*project.ProjectForm), byToken: make(map[string]*project.ProjectForm)}
	for _, form := range fs {
		f.forms[form.ID] = form
		f.byToken[string(form.TokenHash)] = form
	}
	return f
}

func (f *fakeForms) Create(_ context.Context, _ string, form *project.ProjectForm) error {
	f.created = form
	cp := *form
	f.forms[form.ID] = &cp
	f.byToken[string(form.TokenHash)] = &cp
	return nil
}

func (f *fakeForms) Get(_ context.Context, _, id string) (*project.ProjectForm, error) {
	form, ok := f.forms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *form
	return &cp, nil
}

func (f *fakeForms) Update(_ context.Context, _ string, form *project.ProjectForm) error {
	if _, ok := f.forms[form.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *form
	f.forms[form.ID] = &cp
	return nil
}

func (f *fakeForms) RotateToken(_ context.Context, _, id string, hash []byte) error {
	form, ok := f.forms[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(f.byToken, string(form.TokenHash))
	form.TokenHash = hash
	f.byToken[string(hash)] = form
	return nil
}

func (f *fakeForms) ResolveByToken(_ context.Context, hash []byte) (*project.ProjectForm, error) {
	form, ok := f.byToken[string(hash)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *form
	return &cp, nil
}

type fakeFormTasks struct {
	project.TaskRepository
	tasks   []*project.Task
	created *project.Task
}

func (f *fakeFormTasks) Create(_ context.Context, _ string, t *project.Task) error {
	t.Number = len(f.tasks) + 1
	f.created = t
	f.tasks = append(f.tasks, t)
	return nil
}

func (f *fakeFormTasks) Get(_ context.Context, _, _ string) (*project.Task, error) {
	if f.created == nil {
		return nil, domain.ErrNotFound
	}
	return f.created, nil
}

func (f *fakeFormTasks) ListByColumn(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}

type fakeFormBoards struct {
	project.BoardRepository
}

func (f *fakeFormBoards) GetByProject(_ context.Context, _, _ string) (*project.Board, error) {
	return &project.Board{ID: "b1"}, nil
}

type fakeFormColumns struct {
	project.ColumnRepository
}

func (f *fakeFormColumns) Get(_ context.Context, _, id string) (*project.Column, error) {
	return &project.Column{ID: id, BoardID: "b1"}, nil
}

func formTestService(forms *fakeForms, tasks *fakeFormTasks) *Service {
	lead := project.RoleLead
	return New(Deps{
		Projects: &fakeProjects{p: &project.Project{ID: "p1", Visibility: project.VisibilityOrg}},
		Members:  &fakeMembers{role: &lead},
		Boards:   &fakeFormBoards{},
		Columns:  &fakeFormColumns{},
		Tasks:    tasks,
		Forms:    forms,
	})
}

func TestCreateForm_ReturnsOneTimeToken(t *testing.T) {
	ctx := context.Background()
	forms := newFakeForms()
	tasks := &fakeFormTasks{}
	s := formTestService(forms, tasks)

	got, raw, err := s.CreateForm(ctx, "o", "u", "p1", CreateFormInput{
		Name: "Bug reports", TargetColumnID: "c1",
	}, tenant.RoleMember)
	if err != nil {
		t.Fatalf("CreateForm: %v", err)
	}
	if raw == "" {
		t.Fatal("empty raw token")
	}
	if string(forms.created.TokenHash) == raw {
		t.Error("stored hash must not equal the raw token")
	}
	if len(forms.created.TokenHash) != 32 {
		t.Errorf("hash len = %d, want 32 (SHA-256)", len(forms.created.TokenHash))
	}
	if got.Name != "Bug reports" || !got.IsActive {
		t.Errorf("stored = %+v", got)
	}
}

func TestCreateForm_Validation(t *testing.T) {
	ctx := context.Background()
	s := formTestService(newFakeForms(), &fakeFormTasks{})

	for name, in := range map[string]CreateFormInput{
		"empty name": {Name: "  ", TargetColumnID: "c1"},
	} {
		if _, _, err := s.CreateForm(ctx, "o", "u", "p1", in, tenant.RoleMember); err != domain.ErrValidation {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}

func TestSubmitForm_HappyPath(t *testing.T) {
	ctx := context.Background()
	raw := "test-token-1"
	forms := newFakeForms(&project.ProjectForm{
		ID: "f1", OrgID: "o", ProjectID: "p1", Name: "Bugs",
		TargetColumnID: "c1", TokenHash: token.Hash(raw), CreatedBy: "owner-1", IsActive: true,
	})
	tasks := &fakeFormTasks{}
	s := formTestService(forms, tasks)

	got, err := s.SubmitForm(ctx, raw, SubmitFormInput{
		Title: "Login broken", Description: "500 on submit",
		Priority: "high", SubmitterName: "Ada", SubmitterEmail: "ada@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitForm: %v", err)
	}
	if got.Title != "Login broken" || got.Priority != project.PriorityHigh {
		t.Errorf("task = %+v", got)
	}
	if got.CreatedBy != "owner-1" {
		t.Errorf("created_by = %s, want form owner", got.CreatedBy)
	}
	if !strings.Contains(got.Description, "Ada <ada@example.com>") || !strings.Contains(got.Description, "500 on submit") {
		t.Errorf("description missing attribution/body: %q", got.Description)
	}
	if got.Number != 1 {
		t.Errorf("number = %d, want 1", got.Number)
	}
}

func TestSubmitForm_Rejects(t *testing.T) {
	ctx := context.Background()
	activeRaw := "active-token"
	inactiveRaw := "inactive-token"
	forms := newFakeForms(
		&project.ProjectForm{ID: "f1", OrgID: "o", ProjectID: "p1", TargetColumnID: "c1", TokenHash: token.Hash(activeRaw), CreatedBy: "u", IsActive: true},
		&project.ProjectForm{ID: "f2", OrgID: "o", ProjectID: "p1", TargetColumnID: "c1", TokenHash: token.Hash(inactiveRaw), CreatedBy: "u", IsActive: false},
	)
	s := formTestService(forms, &fakeFormTasks{})

	ok := SubmitFormInput{Title: "T"}
	cases := []struct {
		name  string
		token string
		in    SubmitFormInput
		want  error
	}{
		{"unknown token", "nope", ok, domain.ErrNotFound},
		{"inactive form", inactiveRaw, ok, domain.ErrNotFound},
		{"empty title", activeRaw, SubmitFormInput{}, domain.ErrValidation},
		{"bad priority", activeRaw, SubmitFormInput{Title: "T", Priority: "urgent-ish"}, domain.ErrValidation},
		{"bad email", activeRaw, SubmitFormInput{Title: "T", SubmitterEmail: "not-an-email"}, domain.ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.SubmitForm(ctx, c.token, c.in); err != c.want {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestGetPublicForm_InactiveIsNotFound(t *testing.T) {
	ctx := context.Background()
	raw := "off-token"
	forms := newFakeForms(&project.ProjectForm{
		ID: "f1", OrgID: "o", ProjectID: "p1", Name: "X",
		TargetColumnID: "c1", TokenHash: token.Hash(raw), CreatedBy: "u",
	})
	s := formTestService(forms, &fakeFormTasks{})

	if _, err := s.GetPublicForm(ctx, raw); err != domain.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetPublicForm(ctx, "missing"); err != domain.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
