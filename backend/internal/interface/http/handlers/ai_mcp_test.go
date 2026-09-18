package handlers

// MCP dispatch contract (ADR-025, FR-AI-007): unknown tools 422, validation
// errors surface as domain sentinels, happy paths return result maps. Tests
// dispatchMCP directly (tenant ctx lives in the router, not the dispatcher)
// with an aiuc.Service over in-memory fakes + the real mock provider.

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/admin"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	"github.com/mesutokul/fluxboard/backend/internal/domain/audit"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	infraai "github.com/mesutokul/fluxboard/backend/internal/infrastructure/ai"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/aiuc"
)

type mcpRuns struct {
	mu   sync.Mutex
	rows map[string]*ai.Run
}

func (f *mcpRuns) Create(_ context.Context, _ string, r *ai.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows == nil {
		f.rows = map[string]*ai.Run{}
	}
	cp := *r
	f.rows[r.ID] = &cp
	return nil
}

func (f *mcpRuns) Get(_ context.Context, _, id string) (*ai.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *mcpRuns) ListRecent(_ context.Context, _ string, _ int) ([]ai.Run, error) {
	return nil, nil
}

func (f *mcpRuns) CountSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

func (f *mcpRuns) PurgeBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

type mcpRisks struct{}

func (mcpRisks) UpsertOpen(_ context.Context, _ string, _ *ai.Risk) error { return nil }
func (mcpRisks) ListOpen(_ context.Context, _, _ string) ([]ai.Risk, error) {
	return []ai.Risk{}, nil
}
func (mcpRisks) Dismiss(_ context.Context, _, _ string) error { return domain.ErrNotFound }

type mcpFlags struct{}

func (mcpFlags) List(_ context.Context, _ string) ([]admin.FeatureFlag, error) { return nil, nil }
func (mcpFlags) Set(_ context.Context, _, _ string, _ bool) error              { return nil }

type mcpTasks struct{ byID map[string]*project.Task }

func (f *mcpTasks) Get(_ context.Context, _, id string) (*project.Task, error) {
	if t, ok := f.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}

func (f *mcpTasks) ListByProject(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}

func (f *mcpTasks) Create(_ context.Context, _ string, _ *project.Task) error { return nil }
func (f *mcpTasks) ListByColumn(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}
func (f *mcpTasks) Update(_ context.Context, _ string, _ *project.Task) error { return nil }
func (f *mcpTasks) Move(_ context.Context, _, _, _, _ string) error           { return nil }
func (f *mcpTasks) SoftDelete(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (f *mcpTasks) Restore(_ context.Context, _, _ string) error { return nil }
func (f *mcpTasks) GetTrashed(_ context.Context, _, _ string) (*project.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *mcpTasks) ListTrashed(_ context.Context, _, _ string) ([]project.Task, error) {
	return nil, nil
}
func (f *mcpTasks) PurgeExpired(_ context.Context, _ string, _ time.Time) (int, error) {
	return 0, nil
}
func (f *mcpTasks) Search(_ context.Context, _ string, _ project.SearchFilter) (*project.SearchResult, error) {
	return &project.SearchResult{}, nil
}
func (f *mcpTasks) CountLive(_ context.Context, _ string, ids []string) (int, error) {
	return len(ids), nil
}
func (f *mcpTasks) BulkAssign(_ context.Context, _ string, _ []string, _ *string) error {
	return nil
}
func (f *mcpTasks) BulkMove(_ context.Context, _, _ string, _, _ []string) error { return nil }

type mcpProjects struct{}

func (mcpProjects) Get(_ context.Context, _, _ string) (*project.Project, error) {
	return nil, domain.ErrNotFound
}
func (mcpProjects) Create(_ context.Context, _ string, _ *project.Project) error { return nil }
func (mcpProjects) List(_ context.Context, _, _ string, _, _ bool) ([]project.Project, error) {
	return nil, nil
}
func (mcpProjects) Update(_ context.Context, _ string, _ *project.Project) error { return nil }
func (mcpProjects) SetArchived(_ context.Context, _, _ string, _ *time.Time) error {
	return nil
}
func (mcpProjects) NextNumber(_ context.Context, _, _ string) (int, error) { return 1, nil }

type mcpAudit struct{}

func (mcpAudit) Append(_ context.Context, _ audit.Entry) error { return nil }

type mcpIdem struct {
	mu sync.Mutex
	m  map[string]string
}

func (f *mcpIdem) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[key], nil
}

func (f *mcpIdem) Set(_ context.Context, key, val string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]string{}
	}
	f.m[key] = val
	return nil
}

type mcpCreator struct{ n int }

func (f *mcpCreator) CreateTask(_ context.Context, orgID, _ string, in aiuc.TaskCreateInput, _ tenant.OrgRole) (*project.Task, error) {
	f.n++
	return &project.Task{ID: "t-new", OrgID: orgID, ProjectID: in.ProjectID, Number: f.n, Title: in.Title}, nil
}

func (f *mcpCreator) TrashTask(_ context.Context, _, _, _ string, _ tenant.OrgRole) error {
	return nil
}

func newMCPHarness() *AIHandlers {
	svc := aiuc.New(aiuc.Deps{
		Runs: &mcpRuns{}, Risks: mcpRisks{}, Flags: mcpFlags{},
		Tasks: &mcpTasks{byID: map[string]*project.Task{}}, Projects: mcpProjects{},
		Audit: mcpAudit{}, Provider: infraai.NewMock(), Idem: &mcpIdem{},
		Creator: &mcpCreator{},
	})
	return NewAIHandlers(svc, nil)
}

func callMCP(h *AIHandlers, tool string, params map[string]any) (any, error) {
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/orgs/o1/mcp", nil)
	return h.dispatchMCP(req, "o1", "u1", tenant.RoleAdmin, mcpReq{Tool: tool, Params: params})
}

func TestMCPUnknownTool(t *testing.T) {
	h := newMCPHarness()
	if _, err := callMCP(h, "ai.takeover", nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestMCPParse(t *testing.T) {
	h := newMCPHarness()
	res, err := callMCP(h, MCPParse, map[string]any{"input": "ship login. fix logout"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["items"] == nil {
		t.Fatalf("result = %#v", res)
	}
	if _, err := callMCP(h, MCPParse, map[string]any{"input": "   "}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestMCPPlanApply(t *testing.T) {
	h := newMCPHarness()
	params := map[string]any{
		"project_id": "p1", "column_id": "c1", "idempotency_key": "k1",
		"items": []any{
			map[string]any{"title": "one"},
			map[string]any{"title": "two", "priority": "high", "due_date": "2026-10-01T00:00:00Z"},
		},
	}
	res, err := callMCP(h, MCPPlanApply, params)
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["tasks"] == nil || m["run_id"] == nil {
		t.Fatalf("result = %#v", res)
	}
	bad := map[string]any{"project_id": "p1", "column_id": "c1", "items": []any{"nope"}}
	if _, err := callMCP(h, MCPPlanApply, bad); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
	badDate := map[string]any{
		"project_id": "p1", "column_id": "c1",
		"items": []any{map[string]any{"title": "x", "due_date": "yesterday"}},
	}
	if _, err := callMCP(h, MCPPlanApply, badDate); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want validation, got %v", err)
	}
}

func TestMCPNotFoundPaths(t *testing.T) {
	h := newMCPHarness()
	if _, err := callMCP(h, MCPDigest, map[string]any{"project_id": "nope"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("digest: got %v", err)
	}
	if _, err := callMCP(h, MCPRisksDismiss, map[string]any{"task_id": "nope"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("dismiss: got %v", err)
	}
}
