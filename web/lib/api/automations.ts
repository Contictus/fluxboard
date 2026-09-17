import { apiFetch } from './client';

export type AutomationTrigger = 'task.created' | 'task.moved' | 'task.assigned';
export type AutomationAction = 'assign' | 'set_priority' | 'move' | 'add_label';

/** One automation rule (GET /orgs/{id}/automations -> { rules: [...] }). */
export interface AutomationRule {
  id: string;
  name: string;
  enabled: boolean;
  trigger: AutomationTrigger;
  trigger_config: { column_id?: string };
  action: AutomationAction;
  action_config: { user_id?: string; priority?: string; column_id?: string; label_id?: string };
  created_by: string;
  created_at: string;
}

/** Payload for POST/PATCH /orgs/{id}/automations. */
export interface AutomationRuleInput {
  name: string;
  trigger: AutomationTrigger;
  trigger_config: { column_id?: string };
  action: AutomationAction;
  action_config: { user_id?: string; priority?: string; column_id?: string; label_id?: string };
}

export const AUTOMATION_TRIGGERS: { value: AutomationTrigger; label: string; hint: string }[] = [
  { value: 'task.created', label: 'Task created', hint: 'Runs for every new task' },
  { value: 'task.moved', label: 'Task moved', hint: 'Optionally only into one column' },
  { value: 'task.assigned', label: 'Task assigned', hint: 'Runs when an assignee is set' },
];

export const AUTOMATION_ACTIONS: { value: AutomationAction; label: string }[] = [
  { value: 'assign', label: 'Assign to someone' },
  { value: 'set_priority', label: 'Set priority' },
  { value: 'move', label: 'Move to column' },
  { value: 'add_label', label: 'Add label' },
];

/** List rules (ADMIN+ read). */
export async function listAutomationRules(orgId: string): Promise<AutomationRule[]> {
  const res = await apiFetch<{ rules: AutomationRule[] }>(`/orgs/${orgId}/automations`);
  return res.rules ?? [];
}

/** Create a rule (201, ADMIN+). */
export function createAutomationRule(orgId: string, input: AutomationRuleInput): Promise<AutomationRule> {
  return apiFetch(`/orgs/${orgId}/automations`, { method: 'POST', body: input });
}

/** Replace a rule's definition (ADMIN+). */
export function updateAutomationRule(
  orgId: string,
  ruleId: string,
  input: AutomationRuleInput,
): Promise<AutomationRule> {
  return apiFetch(`/orgs/${orgId}/automations/${ruleId}`, { method: 'PATCH', body: input });
}

/** Enable/disable a rule (ADMIN+). */
export function setAutomationRuleEnabled(
  orgId: string,
  ruleId: string,
  enabled: boolean,
): Promise<AutomationRule> {
  return apiFetch(`/orgs/${orgId}/automations/${ruleId}/enabled`, {
    method: 'POST',
    body: { enabled },
  });
}

/** Delete a rule (204, ADMIN+). */
export function deleteAutomationRule(orgId: string, ruleId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/automations/${ruleId}`, { method: 'DELETE' });
}
