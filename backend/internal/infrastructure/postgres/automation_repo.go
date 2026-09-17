// Package postgres implements the infrastructure repositories.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/automation"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// AutomationRepo is the Postgres-backed automation.RuleRepository ([T], RLS).
type AutomationRepo struct {
	tp *TenantPool
}

// NewAutomationRepo builds an AutomationRepo. tp must be the tenant-scoped pool.
func NewAutomationRepo(tp *TenantPool) *AutomationRepo {
	return &AutomationRepo{tp: tp}
}

// Compile-time port check.
var _ automation.RuleRepository = (*AutomationRepo)(nil)

func marshalConfig(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func mapAutomationRule(row gen.AutomationRule) (*automation.Rule, error) {
	r := &automation.Rule{
		ID: row.ID.String(), OrgID: row.OrgID.String(), Name: row.Name,
		Enabled: row.Enabled, Trigger: row.Trigger, Action: row.Action,
		CreatedBy: row.CreatedBy.String(), CreatedAt: row.CreatedAt,
	}
	if err := json.Unmarshal(row.TriggerConfig, &r.TriggerConfig); err != nil {
		return nil, fmt.Errorf("automation: bad trigger_config: %w", err)
	}
	if err := json.Unmarshal(row.ActionConfig, &r.ActionConfig); err != nil {
		return nil, fmt.Errorf("automation: bad action_config: %w", err)
	}
	return r, nil
}

func (r *AutomationRepo) Create(ctx context.Context, orgID string, rule *automation.Rule) error {
	id, err := parseUUID(rule.ID)
	if err != nil {
		return fmt.Errorf("automation create: %w", err)
	}
	by, err := parseUUID(rule.CreatedBy)
	if err != nil {
		return fmt.Errorf("automation create: by: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		return q.CreateAutomationRule(ctx, gen.CreateAutomationRuleParams{
			ID: id, OrgID: oid, Name: rule.Name, Enabled: rule.Enabled,
			Trigger: rule.Trigger, TriggerConfig: marshalConfig(rule.TriggerConfig),
			Action: rule.Action, ActionConfig: marshalConfig(rule.ActionConfig),
			CreatedBy: by,
		})
	})
}

func (r *AutomationRepo) Get(ctx context.Context, orgID, id string) (*automation.Rule, error) {
	rid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("automation get: %w", err)
	}
	var out *automation.Rule
	err = r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		row, err := q.GetAutomationRule(ctx, gen.GetAutomationRuleParams{OrgID: oid, ID: rid})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out, err = mapAutomationRule(row)
		return err
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r *AutomationRepo) list(ctx context.Context, orgID, trigger string, enabledOnly bool) ([]automation.Rule, error) {
	var out []automation.Rule
	out = make([]automation.Rule, 0)
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		var rows []gen.AutomationRule
		var err error
		if enabledOnly {
			rows, err = q.ListEnabledAutomationRules(ctx, gen.ListEnabledAutomationRulesParams{OrgID: oid, Trigger: trigger})
		} else {
			rows, err = q.ListAutomationRules(ctx, oid)
		}
		if err != nil {
			return err
		}
		for _, row := range rows {
			mapped, err := mapAutomationRule(row)
			if err != nil {
				return err
			}
			out = append(out, *mapped)
		}
		return nil
	})
	return out, err
}

// List returns every rule in creation order (settings UI).
func (r *AutomationRepo) List(ctx context.Context, orgID string) ([]automation.Rule, error) {
	return r.list(ctx, orgID, "", false)
}

// ListEnabled returns enabled rules for one trigger (evaluate path).
func (r *AutomationRepo) ListEnabled(ctx context.Context, orgID, trigger string) ([]automation.Rule, error) {
	return r.list(ctx, orgID, trigger, true)
}

func (r *AutomationRepo) Update(ctx context.Context, orgID string, rule *automation.Rule) error {
	rid, err := parseUUID(rule.ID)
	if err != nil {
		return fmt.Errorf("automation update: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.UpdateAutomationRule(ctx, gen.UpdateAutomationRuleParams{
			OrgID: oid, ID: rid, Name: rule.Name, Enabled: rule.Enabled,
			Trigger: rule.Trigger, TriggerConfig: marshalConfig(rule.TriggerConfig),
			Action: rule.Action, ActionConfig: marshalConfig(rule.ActionConfig),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// Delete removes a rule. ErrNotFound if absent.
func (r *AutomationRepo) Delete(ctx context.Context, orgID string, id string) error {
	rid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("automation delete: %w", err)
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		n, err := q.DeleteAutomationRule(ctx, gen.DeleteAutomationRuleParams{OrgID: oid, ID: rid})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
