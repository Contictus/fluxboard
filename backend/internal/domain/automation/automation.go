// Package automation holds the domain types for org-scoped automation rules
// (docs/08, FR-AUTO): "when X happens, do Y" rules evaluated on the task
// write path. Trigger/action vocabularies are closed sets validated in the
// domain so unknown kinds fail fast at rule creation, not at fire time.
package automation

import (
	"context"
	"time"
)

// Triggers.
const (
	TriggerTaskCreated  = "task.created"
	TriggerTaskMoved    = "task.moved"
	TriggerTaskAssigned = "task.assigned"
)

// Actions.
const (
	ActionAssign      = "assign"
	ActionSetPriority = "set_priority"
	ActionMove        = "move"
	ActionAddLabel    = "add_label"
)

// TriggerConfig conditions a trigger. ColumnID applies to TriggerTaskMoved:
// the rule fires only when the task lands in that column.
type TriggerConfig struct {
	ColumnID string `json:"column_id,omitempty"`
}

// ActionConfig carries action parameters. Only the field matching the rule's
// action is read: UserID for assign, Priority for set_priority, ColumnID for
// move, LabelID for add_label.
type ActionConfig struct {
	UserID   string `json:"user_id,omitempty"`
	Priority string `json:"priority,omitempty"`
	ColumnID string `json:"column_id,omitempty"`
	LabelID  string `json:"label_id,omitempty"`
}

// Rule is one automation rule in an organization.
type Rule struct {
	ID            string
	OrgID         string
	Name          string
	Enabled       bool
	Trigger       string
	TriggerConfig TriggerConfig
	Action        string
	ActionConfig  ActionConfig
	CreatedBy     string
	CreatedAt     time.Time
}

// ValidTrigger reports whether t is a known trigger kind.
func ValidTrigger(t string) bool {
	switch t {
	case TriggerTaskCreated, TriggerTaskMoved, TriggerTaskAssigned:
		return true
	}
	return false
}

// ValidAction reports whether a is a known action kind.
func ValidAction(a string) bool {
	switch a {
	case ActionAssign, ActionSetPriority, ActionMove, ActionAddLabel:
		return true
	}
	return false
}

// RuleRepository persists automation rules (FR-AUTO).
type RuleRepository interface {
	Create(ctx context.Context, orgID string, r *Rule) error
	Get(ctx context.Context, orgID, id string) (*Rule, error)
	// List returns every rule in creation order (settings UI).
	List(ctx context.Context, orgID string) ([]Rule, error)
	// ListEnabled returns enabled rules for one trigger (evaluate path).
	ListEnabled(ctx context.Context, orgID, trigger string) ([]Rule, error)
	Update(ctx context.Context, orgID string, r *Rule) error
	// Delete removes a rule. ErrNotFound if absent.
	Delete(ctx context.Context, orgID string, id string) error
}

// Event is one automation-relevant occurrence on the task write path.
type Event struct {
	OrgID      string
	Trigger    string
	TaskID     string
	ActorID    string
	ToColumnID string // task.moved: the destination column
	AssigneeID string // task.assigned: the new assignee ("" when cleared)
}

// Evaluator runs enabled rules for an event. Implemented by the automation
// usecase; taskuc/projectuc depend on this narrow interface (never the
// concrete service) so the dependency rule stays intact.
type Evaluator interface {
	Evaluate(ctx context.Context, e Event)
}
