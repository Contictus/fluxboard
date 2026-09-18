package aiuc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

type fakeCreator struct {
	mu      sync.Mutex
	created []*project.Task
	trashed []string
	failOn  int // 1-based create index to fail; 0 = never fail
	n       int
	nextNum int
}

func (f *fakeCreator) CreateTask(_ context.Context, orgID, _ string, in TaskCreateInput, _ tenant.OrgRole) (*project.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if f.failOn > 0 && f.n == f.failOn {
		return nil, domain.ErrValidation
	}
	f.nextNum++
	t := &project.Task{
		ID: f.nextID(), OrgID: orgID, ProjectID: in.ProjectID, ColumnID: in.ColumnID,
		Number: f.nextNum, Title: in.Title, Description: in.Description,
		AssigneeID: in.AssigneeID, Priority: in.Priority,
	}
	f.created = append(f.created, t)
	return t, nil
}

func (f *fakeCreator) nextID() string { return "t-" + string(rune('0'+f.nextNum)) }

func (f *fakeCreator) TrashTask(_ context.Context, _, _, taskID string, _ tenant.OrgRole) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trashed = append(f.trashed, taskID)
	return nil
}

func applyHarness(flags map[string]bool, cap int, failOn int) (*harness, *fakeCreator) {
	h := newHarness(flags, cap)
	c := &fakeCreator{}
	h.svc.creator = c
	c.failOn = failOn
	return h, c
}

func twoItems() []ApplyItem {
	return []ApplyItem{{Title: "first"}, {Title: "second", Priority: "high"}}
}

func TestApplyOK(t *testing.T) {
	h, c := applyHarness(nil, 0, 0)
	out, err := h.svc.ApplyPlan(context.Background(), "o1", "u1", tenant.RoleAdmin, ApplyInput{
		ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Replayed || len(out.Tasks) != 2 || out.Tasks[0].Number != 1 || out.RunID == "" {
		t.Fatalf("apply = %+v", out)
	}
	if len(c.created) != 2 || len(c.trashed) != 0 {
		t.Fatalf("created=%d trashed=%v", len(c.created), c.trashed)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != audit.ActionAIRun {
		t.Fatalf("audit = %+v", h.audit.entries)
	}
}

func TestApplyValidation(t *testing.T) {
	h, _ := applyHarness(nil, 0, 0)
	ctx := context.Background()
	cases := map[string]ApplyInput{
		"empty items":   {ProjectID: "p1", ColumnID: "c1", Items: nil, IdemKey: "k"},
		"too many":      {ProjectID: "p1", ColumnID: "c1", Items: make([]ApplyItem, MaxApplyItems+1), IdemKey: "k"},
		"empty title":   {ProjectID: "p1", ColumnID: "c1", Items: []ApplyItem{{Title: "  "}}, IdemKey: "k"},
		"bad priority":  {ProjectID: "p1", ColumnID: "c1", Items: []ApplyItem{{Title: "x", Priority: "yolo"}}, IdemKey: "k"},
		"empty key":     {ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "  "},
		"empty project": {ProjectID: "", ColumnID: "c1", Items: twoItems(), IdemKey: "k"},
	}
	for name, in := range cases {
		if _, err := h.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, in); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("%s: want validation, got %v", name, err)
		}
	}
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	bad := ApplyInput{ProjectID: "p1", ColumnID: "c1", Items: []ApplyItem{{Title: "x", StartDate: &future, DueDate: &past}}, IdemKey: "k"}
	if _, err := h.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, bad); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("date inversion: got %v", err)
	}
}

func TestApplyCompensates(t *testing.T) {
	h, c := applyHarness(nil, 0, 2)
	_, err := h.svc.ApplyPlan(context.Background(), "o1", "u1", tenant.RoleAdmin, ApplyInput{
		ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "k1",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	if len(c.trashed) != 1 {
		t.Fatalf("trashed = %v, want the first task", c.trashed)
	}
	h.runs.mu.Lock()
	n := len(h.runs.rows)
	h.runs.mu.Unlock()
	if n != 0 {
		t.Fatalf("failed apply must not ledger, rows = %d", n)
	}
}

func TestApplyReplay(t *testing.T) {
	h, c := applyHarness(nil, 0, 0)
	ctx := context.Background()
	in := ApplyInput{ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "k9"}
	first, err := h.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.RunID != first.RunID || len(second.Tasks) != 2 {
		t.Fatalf("replay = %+v", second)
	}
	if len(c.created) != 2 {
		t.Fatalf("replay created duplicates: %d", len(c.created))
	}
}

func TestApplyGateAndMeter(t *testing.T) {
	h, _ := applyHarness(map[string]bool{ai.FlagPlan: false}, 0, 0)
	in := ApplyInput{ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "k"}
	if _, err := h.svc.ApplyPlan(context.Background(), "o1", "u1", tenant.RoleAdmin, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want forbidden, got %v", err)
	}

	h2, _ := applyHarness(nil, 1, 0)
	ctx := context.Background()
	if _, err := h2.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, in); err != nil {
		t.Fatal(err)
	}
	in2 := in
	in2.IdemKey = "k2"
	if _, err := h2.svc.ApplyPlan(ctx, "o1", "u1", tenant.RoleAdmin, in2); !errors.Is(err, domain.ErrPlanLimit) {
		t.Fatalf("want plan_limit, got %v", err)
	}
}

func TestApplyUnavailable(t *testing.T) {
	h := newHarness(nil, 0) // creator nil
	in := ApplyInput{ProjectID: "p1", ColumnID: "c1", Items: twoItems(), IdemKey: "k"}
	if _, err := h.svc.ApplyPlan(context.Background(), "o1", "u1", tenant.RoleAdmin, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want forbidden, got %v", err)
	}
}
