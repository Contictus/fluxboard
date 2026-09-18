package aiuc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/billing"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	infraai "github.com/mesutokul/fluxboard/backend/internal/infrastructure/ai"
)

// ---- fakes ---------------------------------------------------------------

type fakeRuns struct {
	mu   sync.Mutex
	rows map[string]*ai.Run
}

func newFakeRuns() *fakeRuns { return &fakeRuns{rows: map[string]*ai.Run{}} }

func (f *fakeRuns) Create(_ context.Context, _ string, r *ai.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, old := range f.rows {
		if r.IdempotencyKey != "" && old.IdempotencyKey == r.IdempotencyKey && old.OrgID == r.OrgID {
			return domain.ErrConflict
		}
	}
	cp := *r
	f.rows[r.ID] = &cp
	return nil
}

func (f *fakeRuns) Get(_ context.Context, _, id string) (*ai.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeRuns) ListRecent(_ context.Context, _ string, limit int) ([]ai.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ai.Run, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, *r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRuns) CountSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.rows)), nil
}

func (f *fakeRuns) PurgeBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

type fakeRisks struct {
	mu   sync.Mutex
	open map[string]*ai.Risk // taskID → risk
}

func newFakeRisks() *fakeRisks { return &fakeRisks{open: map[string]*ai.Risk{}} }

func (f *fakeRisks) UpsertOpen(_ context.Context, _ string, r *ai.Risk) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.open[r.TaskID] = &cp
	return nil
}

func (f *fakeRisks) ListOpen(_ context.Context, _, _ string) ([]ai.Risk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ai.Risk, 0, len(f.open))
	for _, r := range f.open {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeRisks) Dismiss(_ context.Context, _, taskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.open[taskID]; !ok {
		return domain.ErrNotFound
	}
	delete(f.open, taskID)
	return nil
}

type fakeFlags struct{ m map[string]bool }

func (f *fakeFlags) List(_ context.Context, _ string) ([]admin.FeatureFlag, error) {
	var out []admin.FeatureFlag
	for k, v := range f.m {
		out = append(out, admin.FeatureFlag{Flag: k, Enabled: v})
	}
	return out, nil
}

func (f *fakeFlags) Set(_ context.Context, _, _ string, _ bool) error { return nil }

type fakeTasks struct {
	byID      map[string]*project.Task
	byProject map[string][]project.Task
}

func (f *fakeTasks) Get(_ context.Context, _, id string) (*project.Task, error) {
	if t, ok := f.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeTasks) ListByProject(_ context.Context, _, projectID string) ([]project.Task, error) {
	return f.byProject[projectID], nil
}

func (f *fakeTasks) Create(_ context.Context, _ string, _ *project.Task) error { return nil }
func (f *fakeTasks) ListByColumn(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}
func (f *fakeTasks) Update(_ context.Context, _ string, _ *project.Task) error    { return nil }
func (f *fakeTasks) Move(_ context.Context, _, _, _, _ string) error              { return nil }
func (f *fakeTasks) SoftDelete(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (f *fakeTasks) Restore(_ context.Context, _, _ string) error                 { return nil }
func (f *fakeTasks) GetTrashed(_ context.Context, _, _ string) (*project.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTasks) ListTrashed(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}
func (f *fakeTasks) PurgeExpired(_ context.Context, _ string, _ time.Time) (int, error) {
	return 0, nil
}
func (f *fakeTasks) Search(_ context.Context, _ string, _ project.SearchFilter) (*project.SearchResult, error) {
	return &project.SearchResult{}, nil
}
func (f *fakeTasks) CountLive(_ context.Context, _ string, ids []string) (int, error) {
	return len(ids), nil
}
func (f *fakeTasks) BulkAssign(_ context.Context, _ string, _ []string, _ *string) error {
	return nil
}
func (f *fakeTasks) BulkMove(_ context.Context, _, _ string, _, _ []string) error {
	return nil
}

type fakeProjects struct{ byID map[string]*project.Project }

func (f *fakeProjects) Get(_ context.Context, _, id string) (*project.Project, error) {
	if p, ok := f.byID[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeProjects) Create(_ context.Context, _ string, _ *project.Project) error { return nil }
func (f *fakeProjects) List(_ context.Context, _, _ string, _, _ bool) ([]project.Project, error) {
	return nil, nil
}
func (f *fakeProjects) Update(_ context.Context, _ string, _ *project.Project) error { return nil }
func (f *fakeProjects) SetArchived(_ context.Context, _, _ string, _ *time.Time) error {
	return nil
}
func (f *fakeProjects) NextNumber(_ context.Context, _, _ string) (int, error) { return 1, nil }

type fakeAudit struct{ entries []audit.Entry }

func (f *fakeAudit) Append(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

type fakeIdem struct {
	mu sync.Mutex
	m  map[string]string
}

func newFakeIdem() *fakeIdem { return &fakeIdem{m: map[string]string{}} }

func (f *fakeIdem) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[key], nil
}

func (f *fakeIdem) Set(_ context.Context, key, val string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[key] = val
	return nil
}

type fakeEnt struct{ plan billing.PlanCode }

func (f *fakeEnt) Resolve(_ context.Context, _ string) (billing.Entitlements, error) {
	return billing.Entitlements{Plan: f.plan}, nil
}

// ---- harness ---------------------------------------------------------------

type harness struct {
	svc   *Service
	runs  *fakeRuns
	risks *fakeRisks
	audit *fakeAudit
	tasks *fakeTasks
}

func newHarness(flags map[string]bool, cap int) *harness {
	runs := newFakeRuns()
	risks := newFakeRisks()
	aud := &fakeAudit{}
	past := time.Now().Add(-48 * time.Hour)
	owner := "u-owner"
	t1 := &project.Task{ID: "t1", OrgID: "o1", ProjectID: "p1", Title: "overdue hot", Priority: project.PriorityUrgent, DueDate: &past}
	t2 := &project.Task{ID: "t2", OrgID: "o1", ProjectID: "p1", Title: "normal", Priority: project.PriorityLow, AssigneeID: &owner}
	tasks := &fakeTasks{
		byID:      map[string]*project.Task{"t1": t1, "t2": t2},
		byProject: map[string][]project.Task{"p1": {*t1, *t2}},
	}
	projs := &fakeProjects{byID: map[string]*project.Project{
		"p1": {ID: "p1", OrgID: "o1", Key: "PAY", Name: "Payments"},
	}}
	svc := New(Deps{
		Runs: runs, Risks: risks, Flags: &fakeFlags{m: flags},
		Tasks: tasks, Projects: projs, Audit: aud,
		Provider: infraai.NewMock(), Idem: newFakeIdem(),
		Entitlements: &fakeEnt{plan: billing.PlanBusiness},
		MonthlyCap:   cap,
	})
	return &harness{svc: svc, runs: runs, risks: risks, audit: aud, tasks: tasks}
}

// ---- tests -----------------------------------------------------------------

func TestParseOK(t *testing.T) {
	h := newHarness(nil, 0)
	out, err := h.svc.Parse(context.Background(), "o1", "u1", "ship login. fix logout")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.RunID == "" {
		t.Fatalf("parse = %+v", out)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != audit.ActionAIRun {
		t.Fatalf("audit = %+v", h.audit.entries)
	}
}

func TestParseValidation(t *testing.T) {
	h := newHarness(nil, 0)
	if _, err := h.svc.Parse(context.Background(), "o1", "u1", "   "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestGateMasterOff(t *testing.T) {
	h := newHarness(map[string]bool{ai.FlagMaster: false}, 0)
	if _, err := h.svc.Parse(context.Background(), "o1", "u1", "do things"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestGateSurfaceOff(t *testing.T) {
	h := newHarness(map[string]bool{ai.FlagChat: false}, 0)
	if _, err := h.svc.Chat(context.Background(), "o1", "u1", "hi", nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want forbidden, got %v", err)
	}
	if _, err := h.svc.Parse(context.Background(), "o1", "u1", "do things"); err != nil {
		t.Fatalf("parse should stay on: %v", err)
	}
}

func TestMeterCap(t *testing.T) {
	h := newHarness(nil, 1)
	if _, err := h.svc.Parse(context.Background(), "o1", "u1", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Parse(context.Background(), "o1", "u1", "second"); !errors.Is(err, domain.ErrPlanLimit) {
		t.Fatalf("want plan_limit, got %v", err)
	}
}

func TestPlanDraftReplay(t *testing.T) {
	h := newHarness(nil, 0)
	ctx := context.Background()
	first, err := h.svc.PlanDraft(ctx, "o1", "u1", "launch payments", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.RunID == "" {
		t.Fatalf("first = %+v", first)
	}
	second, err := h.svc.PlanDraft(ctx, "o1", "u1", "launch payments", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.RunID != first.RunID {
		t.Fatalf("replay = %+v, want run %s", second, first.RunID)
	}
	if _, err := h.svc.PlanDraft(ctx, "o1", "u1", "launch payments", "  "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation for empty key, got %v", err)
	}
}

func TestDigestStats(t *testing.T) {
	h := newHarness(nil, 0)
	out, err := h.svc.Digest(context.Background(), "o1", "u1", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Stats.Total != 2 || out.Stats.Overdue != 1 || out.Stats.Unassigned != 1 || out.Stats.UrgentHigh != 1 {
		t.Fatalf("stats = %+v", out.Stats)
	}
	if out.Text == "" {
		t.Fatal("empty digest text")
	}
	if _, err := h.svc.Digest(context.Background(), "o1", "u1", "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestScanRisksAndDismiss(t *testing.T) {
	h := newHarness(nil, 0)
	ctx := context.Background()
	risks, err := h.svc.ScanRisks(ctx, "o1", "u1", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(risks) != 1 || risks[0].TaskID != "t1" || risks[0].Score != ai.ScoreHigh {
		t.Fatalf("risks = %+v", risks)
	}
	listed, err := h.svc.ListRisks(ctx, "o1", "u1", "p1")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list = %+v, err = %v", listed, err)
	}
	if err := h.svc.DismissRisk(ctx, "o1", "u1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DismissRisk(ctx, "o1", "u1", "t1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not_found on re-dismiss, got %v", err)
	}
	if err := h.svc.DismissRisk(ctx, "o1", "u1", "foreign"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not_found on foreign task, got %v", err)
	}
}

func TestScanRisksPlanGate(t *testing.T) {
	h := newHarness(nil, 0)
	h.svc.entitlements = &fakeEnt{plan: billing.PlanPro}
	if _, err := h.svc.ScanRisks(context.Background(), "o1", "u1", "p1"); !errors.Is(err, domain.ErrPlanLimit) {
		t.Fatalf("want plan_limit, got %v", err)
	}
}

func TestChatBounds(t *testing.T) {
	h := newHarness(nil, 0)
	if _, err := h.svc.Chat(context.Background(), "o1", "u1", "", nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	hist := make([]string, 11)
	for i := range hist {
		hist[i] = "old"
	}
	out, err := h.svc.Chat(context.Background(), "o1", "u1", "hello", hist)
	if err != nil || out.Text == "" {
		t.Fatalf("chat = %+v, err = %v", out, err)
	}
}
