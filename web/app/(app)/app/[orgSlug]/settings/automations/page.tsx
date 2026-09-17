'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Plus, Trash2, Zap } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { memberName, useOrgMembers } from '@/lib/org/use-members';
import { listLabels } from '@/lib/api/labels';
import { listProjects } from '@/lib/api/projects';
import { getBoard } from '@/lib/api/board';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import type { Priority } from '@/lib/api/types';
import {
  AUTOMATION_ACTIONS,
  AUTOMATION_TRIGGERS,
  createAutomationRule,
  deleteAutomationRule,
  listAutomationRules,
  setAutomationRuleEnabled,
  type AutomationAction,
  type AutomationRule,
  type AutomationTrigger,
} from '@/lib/api/automations';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field } from '@/components/auth/field';
import { cn } from '@/lib/utils';

const selectCls =
  'w-full rounded-lg border border-input bg-background px-2 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

// Human sentence for a rule: "When a task is created → assign to X".
function describeRule(
  r: AutomationRule,
  names: { member: (id?: string) => string; label: (id?: string) => string; column: (id?: string) => string },
): string {
  const when =
    r.trigger === 'task.created'
      ? 'a task is created'
      : r.trigger === 'task.moved'
        ? `a task moves${r.trigger_config.column_id ? ` into ${names.column(r.trigger_config.column_id)}` : ''}`
        : 'a task is assigned';
  const then =
    r.action === 'assign'
      ? `assign to ${names.member(r.action_config.user_id)}`
      : r.action === 'set_priority'
        ? `set priority ${priorityLabel((r.action_config.priority ?? 'none') as Priority)}`
        : r.action === 'move'
          ? `move to ${names.column(r.action_config.column_id)}`
          : `add label ${names.label(r.action_config.label_id)}`;
  return `When ${when}, ${then}.`;
}

export default function AutomationsPage() {
  const { orgId } = useOrg();
  const queryClient = useQueryClient();
  const { members, byId } = useOrgMembers();
  const key = ['automations', orgId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const rules = useQuery({ queryKey: key, queryFn: () => listAutomationRules(orgId) });
  const labelsQ = useQuery({ queryKey: ['labels', orgId], queryFn: () => listLabels(orgId) });
  const projectsQ = useQuery({ queryKey: ['projects', orgId], queryFn: () => listProjects(orgId) });
  const labels = labelsQ.data ?? [];
  const projects = projectsQ.data ?? [];

  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [trigger, setTrigger] = useState<AutomationTrigger>('task.created');
  const [triggerColumn, setTriggerColumn] = useState('');
  const [action, setAction] = useState<AutomationAction>('assign');
  const [paramUser, setParamUser] = useState('');
  const [paramPriority, setParamPriority] = useState<Priority>('medium');
  const [paramProject, setParamProject] = useState('');
  const [paramColumn, setParamColumn] = useState('');
  const [paramLabel, setParamLabel] = useState('');

  // Columns for the move target (and the moved-into condition): picked per
  // project because columns live on boards.
  const boardQ = useQuery({
    queryKey: ['board', orgId, paramProject],
    queryFn: () => getBoard(orgId, paramProject),
    enabled: paramProject !== '',
  });
  const boardColumns = boardQ.data?.columns ?? [];

  const createMutation = useMutation({
    mutationFn: () =>
      createAutomationRule(orgId, {
        name: name.trim(),
        trigger,
        trigger_config: trigger === 'task.moved' && triggerColumn ? { column_id: triggerColumn } : {},
        action,
        action_config:
          action === 'assign'
            ? { user_id: paramUser }
            : action === 'set_priority'
              ? { priority: paramPriority }
              : action === 'move'
                ? { column_id: paramColumn }
                : { label_id: paramLabel },
      }),
    onSuccess: () => {
      invalidate();
      setShowForm(false);
      setName('');
      setTriggerColumn('');
      setParamUser('');
      setParamColumn('');
      setParamLabel('');
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (v: { id: string; enabled: boolean }) => setAutomationRuleEnabled(orgId, v.id, v.enabled),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAutomationRule(orgId, id),
    onSuccess: invalidate,
  });

  const canSave =
    name.trim() !== '' &&
    (action !== 'assign' || paramUser !== '') &&
    (action !== 'move' || paramColumn !== '') &&
    (action !== 'add_label' || paramLabel !== '');

  const names = {
    member: (id?: string) => memberName(byId, id),
    label: (id?: string) => labels.find((l) => l.id === id)?.name ?? 'a label',
    column: (id?: string) => {
      if (!id) return 'a column';
      return boardColumns.find((c) => c.id === id)?.name ?? 'a column';
    },
  };

  return (
    <div className="max-w-3xl">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Automations</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            When something happens, do something automatically. Rules run instantly on every
            matching task event.
          </p>
        </div>
        <Button size="sm" onClick={() => setShowForm((v) => !v)}>
          <Plus className="h-4 w-4" /> New rule
        </Button>
      </div>

      {showForm ? (
        <Card className="mt-4">
          <CardHeader>
            <CardTitle>New rule</CardTitle>
            <CardDescription>Pick a trigger, then the action to run.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Field id="rule-name" label="Rule name">
              <input
                id="rule-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Auto-assign new bugs"
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="rule-trigger" label="When">
                <select
                  id="rule-trigger"
                  value={trigger}
                  onChange={(e) => {
                    setTrigger(e.target.value as AutomationTrigger);
                    setTriggerColumn('');
                  }}
                  className={selectCls}
                >
                  {AUTOMATION_TRIGGERS.map((t) => (
                    <option key={t.value} value={t.value} title={t.hint}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field id="rule-action" label="Then">
                <select
                  id="rule-action"
                  value={action}
                  onChange={(e) => setAction(e.target.value as AutomationAction)}
                  className={selectCls}
                >
                  {AUTOMATION_ACTIONS.map((a) => (
                    <option key={a.value} value={a.value}>
                      {a.label}
                    </option>
                  ))}
                </select>
              </Field>
            </div>

            {trigger === 'task.moved' ? (
              <Field id="rule-trigger-col" label="Only when moved into (optional)">
                <select
                  id="rule-trigger-col"
                  value={triggerColumn}
                  onChange={(e) => setTriggerColumn(e.target.value)}
                  className={selectCls}
                >
                  <option value="">Any column</option>
                  {boardColumns.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </Field>
            ) : null}

            {action === 'assign' ? (
              <Field id="rule-assign" label="Assign to">
                <select
                  id="rule-assign"
                  value={paramUser}
                  onChange={(e) => setParamUser(e.target.value)}
                  className={selectCls}
                >
                  <option value="">Select a member…</option>
                  {members.map((m) => (
                    <option key={m.user_id} value={m.user_id}>
                      {m.name || m.email}
                    </option>
                  ))}
                </select>
              </Field>
            ) : null}
            {action === 'set_priority' ? (
              <Field id="rule-priority" label="Priority">
                <select
                  id="rule-priority"
                  value={paramPriority}
                  onChange={(e) => setParamPriority(e.target.value as Priority)}
                  className={selectCls}
                >
                  {PRIORITIES.filter((p) => p !== 'none').map((p) => (
                    <option key={p} value={p}>
                      {priorityLabel(p)}
                    </option>
                  ))}
                </select>
              </Field>
            ) : null}
            {action === 'add_label' ? (
              <Field id="rule-label" label="Label">
                <select
                  id="rule-label"
                  value={paramLabel}
                  onChange={(e) => setParamLabel(e.target.value)}
                  className={selectCls}
                >
                  <option value="">Select a label…</option>
                  {labels.map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.name}
                    </option>
                  ))}
                </select>
              </Field>
            ) : null}
            {action === 'move' || trigger === 'task.moved' ? (
              <div className="grid gap-4 sm:grid-cols-2">
                <Field id="rule-project" label="Project (for columns)">
                  <select
                    id="rule-project"
                    value={paramProject}
                    onChange={(e) => {
                      setParamProject(e.target.value);
                      setParamColumn('');
                      setTriggerColumn('');
                    }}
                    className={selectCls}
                  >
                    <option value="">Select a project…</option>
                    {projects.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </Field>
                {action === 'move' ? (
                  <Field id="rule-move-col" label="Move to column">
                    <select
                      id="rule-move-col"
                      value={paramColumn}
                      onChange={(e) => setParamColumn(e.target.value)}
                      className={selectCls}
                      disabled={!paramProject}
                    >
                      <option value="">{paramProject ? 'Select a column…' : 'Pick a project first'}</option>
                      {boardColumns.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  </Field>
                ) : null}
              </div>
            ) : null}

            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={!canSave || createMutation.isPending}
                onClick={() => createMutation.mutate()}
              >
                {createMutation.isPending ? 'Saving…' : 'Create rule'}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setShowForm(false)}>
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="mt-6 space-y-2">
        {rules.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading rules…</p>
        ) : (rules.data ?? []).length === 0 ? (
          <Card className="p-8 text-center">
            <Zap className="mx-auto h-8 w-8 text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">No automations yet</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Example: when a task is created, assign it to yourself.
            </p>
          </Card>
        ) : (
          (rules.data ?? []).map((r) => (
            <Card key={r.id} className={cn(!r.enabled && 'opacity-60')}>
              <CardContent className="flex items-center gap-3 py-3">
                <button
                  type="button"
                  role="switch"
                  aria-checked={r.enabled}
                  aria-label={`${r.enabled ? 'Disable' : 'Enable'} ${r.name}`}
                  onClick={() => toggleMutation.mutate({ id: r.id, enabled: !r.enabled })}
                  className={cn(
                    'relative h-5 w-9 shrink-0 rounded-full transition-colors',
                    r.enabled ? 'bg-primary' : 'bg-secondary',
                  )}
                >
                  <span
                    className={cn(
                      'absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all',
                      r.enabled ? 'left-[18px]' : 'left-0.5',
                    )}
                  />
                </button>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{r.name}</p>
                  <p className="truncate text-xs text-muted-foreground">{describeRule(r, names)}</p>
                </div>
                <button
                  type="button"
                  onClick={() => deleteMutation.mutate(r.id)}
                  aria-label={`Delete ${r.name}`}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-destructive"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </CardContent>
            </Card>
          ))
        )}
      </div>
    </div>
  );
}
