package projectuc

import (
	"context"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

type fakeSprints struct {
	project.SprintRepository
	sprints map[string]*project.Sprint
	cleared []string
	set     map[string]string
}

func newFakeSprints(sp ...*project.Sprint) *fakeSprints {
	m := make(map[string]*project.Sprint)
	for _, s := range sp {
		m[s.ID] = s
	}
	return &fakeSprints{sprints: m, set: make(map[string]string)}
}

func (f *fakeSprints) Get(_ context.Context, _, id string) (*project.Sprint, error) {
	s, ok := f.sprints[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSprints) GetActive(_ context.Context, _, _ string) (*project.Sprint, error) {
	for _, s := range f.sprints {
		if s.Status == project.SprintActive {
			cp := *s
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSprints) Update(_ context.Context, _ string, s *project.Sprint) error {
	cp := *s
	f.sprints[s.ID] = &cp
	return nil
}

func (f *fakeSprints) SetTaskSprint(_ context.Context, _, taskID string, sprintID *string) error {
	if sprintID == nil {
		delete(f.set, taskID)
		f.cleared = append(f.cleared, taskID)
		return nil
	}
	f.set[taskID] = *sprintID
	return nil
}

type fakeSprintTasks struct {
	project.TaskRepository
	tasks []*project.Task
}

func (f *fakeSprintTasks) Get(_ context.Context, _, id string) (*project.Task, error) {
	for _, t := range f.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSprintTasks) ListByProject(_ context.Context, _, _ string) ([]project.Task, error) {
	out := make([]project.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		out = append(out, *t)
	}
	return out, nil
}

type fakeSprintBoards struct {
	project.BoardRepository
}

func (f *fakeSprintBoards) GetByProject(_ context.Context, _, _ string) (*project.Board, error) {
	return &project.Board{ID: "b1"}, nil
}

type fakeSprintColumns struct {
	project.ColumnRepository
}

func (f *fakeSprintColumns) Get(_ context.Context, _, id string) (*project.Column, error) {
	return &project.Column{ID: id, BoardID: "b1"}, nil
}

func (f *fakeSprintColumns) ListByBoard(_ context.Context, _, _ string) ([]project.Column, error) {
	return []project.Column{{ID: "c1", BoardID: "b1", Rank: "a"}, {ID: "c2", BoardID: "b1", Rank: "b"}}, nil
}

func sprintTestService(sprints *fakeSprints, tasks ...*project.Task) *Service {
	lead := project.RoleLead
	return New(Deps{
		Projects: &fakeProjects{p: &project.Project{ID: "p1", Visibility: project.VisibilityOrg}},
		Members:  &fakeMembers{role: &lead},
		Boards:   &fakeSprintBoards{},
		Columns:  &fakeSprintColumns{},
		Tasks:    &fakeSprintTasks{tasks: tasks},
		Sprints:  sprints,
	})
}

func TestStartSprint_ConflictWhenActive(t *testing.T) {
	ctx := context.Background()
	sprints := newFakeSprints(
		&project.Sprint{ID: "s-active", ProjectID: "p1", Status: project.SprintActive},
		&project.Sprint{ID: "s-new", ProjectID: "p1", Status: project.SprintPlanned},
	)
	s := sprintTestService(sprints)
	if _, err := s.StartSprint(ctx, "o", "u", "p1", "s-new", tenant.RoleMember); err != domain.ErrConflict {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestCompleteSprint_SnapshotsAndBacklogs(t *testing.T) {
	ctx := context.Background()
	sid := "s1"
	sprints := newFakeSprints(&project.Sprint{ID: sid, ProjectID: "p1", Status: project.SprintActive})
	s := sprintTestService(sprints,
		&project.Task{ID: "t-done", ProjectID: "p1", ColumnID: "c2", SprintID: &sid},
		&project.Task{ID: "t-open", ProjectID: "p1", ColumnID: "c1", SprintID: &sid},
		&project.Task{ID: "t-backlog", ProjectID: "p1", ColumnID: "c1"},
	)
	got, err := s.CompleteSprint(ctx, "o", "u", "p1", sid, tenant.RoleMember)
	if err != nil {
		t.Fatalf("CompleteSprint: %v", err)
	}
	if got.Status != project.SprintCompleted {
		t.Errorf("status = %s, want completed", got.Status)
	}
	if got.CompletedTotal != 2 || got.CompletedDone != 1 {
		t.Errorf("snapshot = %d/%d, want 2/1", got.CompletedDone, got.CompletedTotal)
	}
	if len(sprints.cleared) != 1 || sprints.cleared[0] != "t-open" {
		t.Errorf("backlogged = %v, want [t-open]", sprints.cleared)
	}
}
