// Package automationuc holds the application service for automation rules
// (docs/08, FR-AUTO). Rules are evaluated synchronously on the task write
// path: taskuc (create/update) and projectuc (move) call Evaluate
// best-effort after publishing their realtime event. The service depends
// only on domain ports; actions write through the repositories directly so
// automation side effects never re-trigger evaluation (no loops).
package automationuc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/rank"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// Deps are the collaborators the service needs.
type Deps struct {
	Rules   automation.RuleRepository
	Tasks   project.TaskRepository
	Labels  project.LabelRepository
	Columns project.ColumnRepository
	Boards  project.BoardRepository
	Logger  *slog.Logger
}

// Service implements automation rule CRUD and event evaluation.
type Service struct {
	rules   automation.RuleRepository
	tasks   project.TaskRepository
	labels  project.LabelRepository
	columns project.ColumnRepository
	boards  project.BoardRepository
	logger  *slog.Logger
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		rules: d.Rules, tasks: d.Tasks, labels: d.Labels,
		columns: d.Columns, boards: d.Boards, logger: logger,
	}
}

// ---- CRUD (ADMIN+, gated in the handler) ------------------------------------

// CreateInput carries the rule builder form.
type CreateInput struct {
	Name          string
	Trigger       string
	TriggerConfig automation.TriggerConfig
	Action        string
	ActionConfig  automation.ActionConfig
	CreatedBy     string
}

// Create validates the vocabulary and stores the rule.
func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (*automation.Rule, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrValidation)
	}
	if !automation.ValidTrigger(in.Trigger) {
		return nil, fmt.Errorf("%w: unknown trigger", domain.ErrValidation)
	}
	if !automation.ValidAction(in.Action) {
		return nil, fmt.Errorf("%w: unknown action", domain.ErrValidation)
	}
	if err := checkActionConfig(in.Action, in.ActionConfig); err != nil {
		return nil, err
	}
	r := &automation.Rule{
		ID: uuidv7.New().String(), OrgID: orgID, Name: strings.TrimSpace(in.Name),
		Enabled: true, Trigger: in.Trigger, TriggerConfig: in.TriggerConfig,
		Action: in.Action, ActionConfig: in.ActionConfig, CreatedBy: in.CreatedBy,
	}
	if err := s.rules.Create(ctx, orgID, r); err != nil {
		return nil, err
	}
	return s.rules.Get(ctx, orgID, r.ID)
}

// List returns every rule in creation order (settings UI).
func (s *Service) List(ctx context.Context, orgID string) ([]automation.Rule, error) {
	return s.rules.List(ctx, orgID)
}

// Update replaces a rule's definition. Unknown trigger/action rejected.
func (s *Service) Update(ctx context.Context, orgID, id string, in CreateInput) (*automation.Rule, error) {
	r, err := s.rules.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name required", domain.ErrValidation)
	}
	if !automation.ValidTrigger(in.Trigger) {
		return nil, fmt.Errorf("%w: unknown trigger", domain.ErrValidation)
	}
	if !automation.ValidAction(in.Action) {
		return nil, fmt.Errorf("%w: unknown action", domain.ErrValidation)
	}
	if err := checkActionConfig(in.Action, in.ActionConfig); err != nil {
		return nil, err
	}
	r.Name = strings.TrimSpace(in.Name)
	r.Trigger = in.Trigger
	r.TriggerConfig = in.TriggerConfig
	r.Action = in.Action
	r.ActionConfig = in.ActionConfig
	if err := s.rules.Update(ctx, orgID, r); err != nil {
		return nil, err
	}
	return r, nil
}

// SetEnabled flips a rule without touching its definition.
func (s *Service) SetEnabled(ctx context.Context, orgID, id string, enabled bool) (*automation.Rule, error) {
	r, err := s.rules.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled
	if err := s.rules.Update(ctx, orgID, r); err != nil {
		return nil, err
	}
	return r, nil
}

// Delete removes a rule.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.rules.Delete(ctx, orgID, id)
}

func checkActionConfig(action string, c automation.ActionConfig) error {
	switch action {
	case automation.ActionAssign:
		if strings.TrimSpace(c.UserID) == "" {
			return fmt.Errorf("%w: assign needs user_id", domain.ErrValidation)
		}
	case automation.ActionSetPriority:
		if !project.Priority(c.Priority).Valid() || c.Priority == string(project.PriorityNone) {
			return fmt.Errorf("%w: set_priority needs a real priority", domain.ErrValidation)
		}
	case automation.ActionMove:
		if strings.TrimSpace(c.ColumnID) == "" {
			return fmt.Errorf("%w: move needs column_id", domain.ErrValidation)
		}
	case automation.ActionAddLabel:
		if strings.TrimSpace(c.LabelID) == "" {
			return fmt.Errorf("%w: add_label needs label_id", domain.ErrValidation)
		}
	}
	return nil
}

// ---- Evaluate ---------------------------------------------------------------

// Evaluate runs enabled rules for e.Trigger against the task. Best-effort:
// per-rule failures are logged and skipped, never returned, so automation
// can never fail the write that triggered it.
func (s *Service) Evaluate(ctx context.Context, e automation.Event) {
	if s.rules == nil {
		return
	}
	rules, err := s.rules.ListEnabled(ctx, e.OrgID, e.Trigger)
	if err != nil {
		s.logger.Warn("automation list failed", "err", err, "org", e.OrgID, "trigger", e.Trigger)
		return
	}
	if len(rules) == 0 {
		return
	}
	t, err := s.tasks.Get(ctx, e.OrgID, e.TaskID)
	if err != nil {
		s.logger.Warn("automation task load failed", "err", err, "task", e.TaskID)
		return
	}
	for i := range rules {
		r := &rules[i]
		if !matchTrigger(r, e) {
			continue
		}
		if err := s.apply(ctx, e.OrgID, r, t); err != nil {
			s.logger.Warn("automation rule failed", "err", err, "rule", r.ID, "task", e.TaskID)
		}
	}
}

func matchTrigger(r *automation.Rule, e automation.Event) bool {
	if r.Trigger == automation.TriggerTaskMoved && r.TriggerConfig.ColumnID != "" {
		return r.TriggerConfig.ColumnID == e.ToColumnID
	}
	return true
}

func (s *Service) apply(ctx context.Context, orgID string, r *automation.Rule, t *project.Task) error {
	switch r.Action {
	case automation.ActionAssign:
		uid := strings.TrimSpace(r.ActionConfig.UserID)
		if uid == "" {
			return fmt.Errorf("assign: empty user_id")
		}
		t.AssigneeID = &uid
		return s.tasks.Update(ctx, orgID, t)
	case automation.ActionSetPriority:
		p := project.Priority(r.ActionConfig.Priority)
		if !p.Valid() {
			return fmt.Errorf("set_priority: bad priority")
		}
		t.Priority = p
		return s.tasks.Update(ctx, orgID, t)
	case automation.ActionMove:
		return s.moveTask(ctx, orgID, r.ActionConfig.ColumnID, t)
	case automation.ActionAddLabel:
		lid := strings.TrimSpace(r.ActionConfig.LabelID)
		if lid == "" {
			return fmt.Errorf("add_label: empty label_id")
		}
		return s.labels.Attach(ctx, orgID, t.ID, lid)
	default:
		return fmt.Errorf("unknown action %q", r.Action)
	}
}

// moveTask relocates t into columnID on the same board, minting a fresh rank.
func (s *Service) moveTask(ctx context.Context, orgID, columnID string, t *project.Task) error {
	col, err := s.columns.Get(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	board, err := s.boards.GetByProject(ctx, orgID, t.ProjectID)
	if err != nil {
		return err
	}
	if col.BoardID != board.ID {
		return fmt.Errorf("%w: column is on another board", domain.ErrValidation)
	}
	if col.ID == t.ColumnID {
		return nil // already there
	}
	existing, err := s.tasks.ListByColumn(ctx, orgID, columnID)
	if err != nil {
		return err
	}
	last := ""
	if n := len(existing); n > 0 {
		last = existing[n-1].Rank
	}
	return s.tasks.Move(ctx, orgID, t.ID, columnID, rank.Append(last))
}
