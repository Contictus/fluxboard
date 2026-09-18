package aiuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
)

// MaxApplyItems bounds one plan-apply (blast radius, FR-AI-009).
const MaxApplyItems = 50

// TaskCreateInput mirrors one plan item as a task write. It duplicates
// taskuc.CreateTaskInput field-for-field so aiuc never imports the taskuc
// package (clean layering: usecase depends on domain + pkg only). The cmd
// adapter translates it across.
type TaskCreateInput struct {
	ProjectID   string
	ColumnID    string
	Title       string
	Description string
	AssigneeID  *string
	Priority    project.Priority
	StartDate   *time.Time
	DueDate     *time.Time
}

// TaskCreator is the narrow task-write port apply needs. Implemented by an
// adapter over taskuc.Service in cmd (wired in main.go); faked in tests.
type TaskCreator interface {
	CreateTask(ctx context.Context, orgID, userID string, in TaskCreateInput, orgRole tenant.OrgRole) (*project.Task, error)
	TrashTask(ctx context.Context, orgID, userID, taskID string, orgRole tenant.OrgRole) error
}

// ApplyItem is one draft line to materialize.
type ApplyItem struct {
	Title       string
	Description string
	AssigneeID  *string
	Priority    string
	StartDate   *time.Time
	DueDate     *time.Time
}

// ApplyInput carries the apply target + idempotency.
type ApplyInput struct {
	ProjectID string
	ColumnID  string
	Items     []ApplyItem
	IdemKey   string
}

// AppliedTask is one created task pointer.
type AppliedTask struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// ApplyOut is the apply result.
type ApplyOut struct {
	RunID    string
	Replayed bool
	Tasks    []AppliedTask
}

// ApplyPlan materializes draft items as tasks (FR-AI-009): pre-validate cheap
// rules before the first write, create sequentially through taskuc (numbering,
// ranks, events, automation inherited), and trash what was created if any item
// fails — the board never holds partial state. IdemKey is required; a retry
// returns the first apply.
func (s *Service) ApplyPlan(ctx context.Context, orgID, userID string, orgRole tenant.OrgRole, in ApplyInput) (*ApplyOut, error) {
	if s.creator == nil {
		return nil, fmt.Errorf("%w: plan apply unavailable", domain.ErrForbidden)
	}
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.ColumnID) == "" {
		return nil, fmt.Errorf("%w: project_id and column_id required", domain.ErrValidation)
	}
	if len(in.Items) == 0 || len(in.Items) > MaxApplyItems {
		return nil, fmt.Errorf("%w: items 1..50", domain.ErrValidation)
	}
	if strings.TrimSpace(in.IdemKey) == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key required", domain.ErrValidation)
	}
	inputs, err := prevalidateApply(in)
	if err != nil {
		return nil, err
	}
	if err := s.pipeline(ctx, orgID, ai.KindPlanApply); err != nil {
		return nil, err
	}
	key := "ai:idem:" + orgID + ":apply:" + strings.TrimSpace(in.IdemKey)
	if s.idem != nil {
		if priorID, err := s.idem.Get(ctx, key); err == nil && priorID != "" {
			if run, err := s.runs.Get(ctx, orgID, priorID); err == nil {
				return &ApplyOut{RunID: run.ID, Replayed: true, Tasks: appliedFrom(run)}, nil
			}
		}
	}
	var created []*project.Task
	for i := range inputs {
		t, err := s.creator.CreateTask(ctx, orgID, userID, inputs[i], orgRole)
		if err != nil {
			s.compensate(ctx, orgID, userID, orgRole, created)
			return nil, err
		}
		created = append(created, t)
	}
	tasks := make([]AppliedTask, 0, len(created))
	for _, t := range created {
		tasks = append(tasks, AppliedTask{ID: t.ID, Number: t.Number, Title: t.Title})
	}
	out, _ := json.Marshal(map[string]any{"task_ids": tasks, "count": len(tasks)})
	res := ai.CompleteResponse{Model: "plan-apply", PromptTokens: 0, CompletionTokens: 0}
	runID := s.newRun(ctx, orgID, userID, ai.KindPlanApply, describeApply(in), strings.TrimSpace(in.IdemKey), res, out)
	if s.idem != nil {
		_ = s.idem.Set(ctx, key, runID, IdempotencyTTL)
	}
	return &ApplyOut{RunID: runID, Tasks: tasks}, nil
}

// prevalidateApply enforces the cheap rules before any write: title bounds
// (mirrors CreateTask), known priorities, sane date ranges.
func prevalidateApply(in ApplyInput) ([]TaskCreateInput, error) {
	out := make([]TaskCreateInput, 0, len(in.Items))
	for i := range in.Items {
		it := &in.Items[i]
		title := strings.TrimSpace(it.Title)
		if title == "" || len(title) > project.MaxTitleLen {
			return nil, fmt.Errorf("%w: item %d title 1..200 chars", domain.ErrValidation, i)
		}
		priority := project.Priority(strings.TrimSpace(it.Priority))
		if priority == "" {
			priority = project.PriorityNone
		}
		if !priority.Valid() {
			return nil, fmt.Errorf("%w: item %d unknown priority", domain.ErrValidation, i)
		}
		if it.StartDate != nil && it.DueDate != nil && it.StartDate.After(*it.DueDate) {
			return nil, fmt.Errorf("%w: item %d start after due", domain.ErrValidation, i)
		}
		out = append(out, TaskCreateInput{
			ProjectID: in.ProjectID, ColumnID: in.ColumnID,
			Title: title, Description: it.Description, AssigneeID: it.AssigneeID,
			Priority: priority, StartDate: it.StartDate, DueDate: it.DueDate,
		})
	}
	return out, nil
}

// compensate trashes created tasks best-effort after a mid-apply failure.
// Residue lands in Trash (restorable, 30d purge) — never on the board.
func (s *Service) compensate(ctx context.Context, orgID, userID string, orgRole tenant.OrgRole, created []*project.Task) {
	for _, t := range created {
		if err := s.creator.TrashTask(ctx, orgID, userID, t.ID, orgRole); err != nil {
			s.logger.Warn("ai apply compensation failed", "err", err, "task", t.ID)
		}
	}
}

// describeApply summarizes the apply for the ledger input (no full dump).
func describeApply(in ApplyInput) string {
	return fmt.Sprintf("apply %d items to project %s column %s", len(in.Items), in.ProjectID, in.ColumnID)
}

// appliedFrom rebuilds the task list from a stored apply run (replay path).
func appliedFrom(run *ai.Run) []AppliedTask {
	var doc struct {
		TaskIDs []AppliedTask `json:"task_ids"`
	}
	if err := json.Unmarshal(run.Output, &doc); err != nil {
		return []AppliedTask{}
	}
	if doc.TaskIDs == nil {
		return []AppliedTask{}
	}
	return doc.TaskIDs
}
