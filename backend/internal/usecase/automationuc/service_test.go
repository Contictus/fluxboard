package automationuc

import (
	"context"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
)

type fakeRules struct {
	automation.RuleRepository
	rules []automation.Rule
}

func (f *fakeRules) Create(_ context.Context, _ string, r *automation.Rule) error {
	f.rules = append(f.rules, *r)
	return nil
}

func (f *fakeRules) Get(_ context.Context, _, id string) (*automation.Rule, error) {
	for i := range f.rules {
		if f.rules[i].ID == id {
			return &f.rules[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeRules) List(_ context.Context, _ string) ([]automation.Rule, error) {
	return f.rules, nil
}

func (f *fakeRules) ListEnabled(_ context.Context, _ string, trigger string) ([]automation.Rule, error) {
	var out []automation.Rule
	for _, r := range f.rules {
		if r.Enabled && r.Trigger == trigger {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRules) Update(_ context.Context, _ string, r *automation.Rule) error {
	for i := range f.rules {
		if f.rules[i].ID == r.ID {
			f.rules[i] = *r
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeRules) Delete(_ context.Context, _, id string) error {
	for i := range f.rules {
		if f.rules[i].ID == id {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeTasks struct {
	project.TaskRepository
	tasks map[string]*project.Task
	moved []string
}

func (f *fakeTasks) Get(_ context.Context, _, id string) (*project.Task, error) {
	t, ok := f.tasks[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTasks) Update(_ context.Context, _ string, t *project.Task) error {
	cp := *t
	f.tasks[t.ID] = &cp
	return nil
}

func (f *fakeTasks) Move(_ context.Context, _, id, columnID, rank string) error {
	t := f.tasks[id]
	t.ColumnID = columnID
	t.Rank = rank
	f.moved = append(f.moved, id+":"+columnID)
	return nil
}

func (f *fakeTasks) ListByColumn(_ context.Context, _ string, _ string) ([]project.Task, error) {
	return nil, nil
}

type fakeLabels struct {
	project.LabelRepository
	attached [][2]string
}

func (f *fakeLabels) Attach(_ context.Context, _, taskID, labelID string) error {
	f.attached = append(f.attached, [2]string{taskID, labelID})
	return nil
}

type fakeColumns struct {
	project.ColumnRepository
	boardID string
}

func (f *fakeColumns) Get(_ context.Context, _, id string) (*project.Column, error) {
	return &project.Column{ID: id, BoardID: f.boardID}, nil
}

type fakeBoards struct {
	project.BoardRepository
	boardID string
}

func (f *fakeBoards) GetByProject(_ context.Context, _, _ string) (*project.Board, error) {
	return &project.Board{ID: f.boardID}, nil
}

func testService() (*Service, *fakeRules, *fakeTasks, *fakeLabels) {
	rules := &fakeRules{}
	tasks := &fakeTasks{tasks: map[string]*project.Task{
		"t1": {ID: "t1", ProjectID: "p1", ColumnID: "c1", Title: "T", Priority: project.PriorityNone},
	}}
	labels := &fakeLabels{}
	svc := New(Deps{
		Rules: rules, Tasks: tasks, Labels: labels,
		Columns: &fakeColumns{boardID: "b1"}, Boards: &fakeBoards{boardID: "b1"},
	})
	return svc, rules, tasks, labels
}

func TestCreate_RejectsUnknownVocabulary(t *testing.T) {
	svc, _, _, _ := testService()
	if _, err := svc.Create(context.Background(), "o", CreateInput{Name: "x", Trigger: "nope", Action: automation.ActionAssign, ActionConfig: automation.ActionConfig{UserID: "u"}}); err == nil {
		t.Fatal("want validation error for unknown trigger")
	}
	if _, err := svc.Create(context.Background(), "o", CreateInput{Name: "x", Trigger: automation.TriggerTaskCreated, Action: "nope"}); err == nil {
		t.Fatal("want validation error for unknown action")
	}
	if _, err := svc.Create(context.Background(), "o", CreateInput{Name: "x", Trigger: automation.TriggerTaskCreated, Action: automation.ActionAssign}); err == nil {
		t.Fatal("want validation error for assign without user_id")
	}
}

func TestEvaluate_AssignOnCreate(t *testing.T) {
	svc, rules, tasks, _ := testService()
	rules.rules = []automation.Rule{{
		ID: "r1", OrgID: "o", Name: "auto-assign", Enabled: true,
		Trigger: automation.TriggerTaskCreated, Action: automation.ActionAssign,
		ActionConfig: automation.ActionConfig{UserID: "u9"},
	}}
	svc.Evaluate(context.Background(), automation.Event{OrgID: "o", Trigger: automation.TriggerTaskCreated, TaskID: "t1", ActorID: "actor"})
	if tasks.tasks["t1"].AssigneeID == nil || *tasks.tasks["t1"].AssigneeID != "u9" {
		t.Fatalf("assignee not set by rule: %+v", tasks.tasks["t1"].AssigneeID)
	}
}

func TestEvaluate_MoveColumnFilter(t *testing.T) {
	svc, rules, tasks, _ := testService()
	rules.rules = []automation.Rule{{
		ID: "r1", OrgID: "o", Name: "to-done", Enabled: true,
		Trigger: automation.TriggerTaskMoved, TriggerConfig: automation.TriggerConfig{ColumnID: "c2"},
		Action: automation.ActionMove, ActionConfig: automation.ActionConfig{ColumnID: "c3"},
	}}
	// Wrong destination: no-op.
	svc.Evaluate(context.Background(), automation.Event{OrgID: "o", Trigger: automation.TriggerTaskMoved, TaskID: "t1", ActorID: "a", ToColumnID: "c9"})
	if len(tasks.moved) != 0 {
		t.Fatalf("rule fired for non-matching column: %v", tasks.moved)
	}
	// Matching destination: moves onward.
	svc.Evaluate(context.Background(), automation.Event{OrgID: "o", Trigger: automation.TriggerTaskMoved, TaskID: "t1", ActorID: "a", ToColumnID: "c2"})
	if len(tasks.moved) != 1 || tasks.tasks["t1"].ColumnID != "c3" {
		t.Fatalf("rule did not move: moved=%v col=%s", tasks.moved, tasks.tasks["t1"].ColumnID)
	}
}

func TestEvaluate_DisabledRuleSkipped(t *testing.T) {
	svc, rules, tasks, _ := testService()
	rules.rules = []automation.Rule{{
		ID: "r1", OrgID: "o", Name: "off", Enabled: false,
		Trigger: automation.TriggerTaskCreated, Action: automation.ActionSetPriority,
		ActionConfig: automation.ActionConfig{Priority: "high"},
	}}
	svc.Evaluate(context.Background(), automation.Event{OrgID: "o", Trigger: automation.TriggerTaskCreated, TaskID: "t1", ActorID: "a"})
	if tasks.tasks["t1"].Priority != project.PriorityNone {
		t.Fatalf("disabled rule fired: %s", tasks.tasks["t1"].Priority)
	}
}
