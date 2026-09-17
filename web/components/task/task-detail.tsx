'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';

import { getTask, listTaskLabels, attachLabel, detachLabel, trashTask, updateTask } from '@/lib/api/tasks';
import { listLabels } from '@/lib/api/labels';
import { useOrg } from '@/lib/org/context';
import { useOrgMembers } from '@/lib/org/use-members';
import { useToast } from '@/components/ui/toast';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import type { Priority, Task, UpdateTaskInput } from '@/lib/api/types';
import { Subtasks } from './subtasks';
import { Comments } from './comments';
import { TaskLinks } from './links';
import { TimeTracker } from './time-tracker';
import { TaskCustomValues } from './custom-values';
import { Attachments } from './attachments';
import { ActivityTimeline } from './activity-timeline';

// Full task detail (FR-TASK-001/002). Inline-edits every field; UpdateTask
// replaces the whole field set, so each change is merged onto the current task
// and sent as a unit. Subtasks/comments/attachments/activity manage their own
// queries. A GUEST sees a read-only view (the server enforces the same).
function toInput(t: Task, patch: Partial<UpdateTaskInput>): UpdateTaskInput {
  return {
    title: t.title,
    description: t.description,
    assignee_id: t.assignee_id ?? null,
    priority: t.priority as Priority,
    start_date: t.start_date ?? null,
    due_date: t.due_date ?? null,
    ...patch,
  };
}

function dateInputValue(iso?: string): string {
  if (!iso) return '';
  return new Date(iso).toISOString().slice(0, 10);
}

export function TaskDetail({
  taskId,
  projectId,
  projectKey,
}: {
  taskId: string;
  projectId: string;
  projectKey: string;
}) {
  const { orgId, slug, role } = useOrg();
  const { members } = useOrgMembers();
  const { toast } = useToast();
  const router = useRouter();
  const qc = useQueryClient();
  const canEdit = role !== 'GUEST';

  const taskKey = ['task', orgId, taskId] as const;
  const { data: task, isLoading, isError } = useQuery({ queryKey: taskKey, queryFn: () => getTask(orgId, taskId) });

  const labelKey = ['task-labels', orgId, taskId] as const;
  const { data: taskLabels } = useQuery({ queryKey: labelKey, queryFn: () => listTaskLabels(orgId, taskId) });
  const { data: orgLabels } = useQuery({ queryKey: ['labels', orgId], queryFn: () => listLabels(orgId) });

  const [titleDraft, setTitleDraft] = useState('');
  const [descDraft, setDescDraft] = useState('');
  const [descDirty, setDescDirty] = useState(false);

  useEffect(() => {
    if (task) {
      setTitleDraft(task.title);
      if (!descDirty) setDescDraft(task.description);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [task?.id, task?.title, task?.description]);

  const patch = useMutation({
    mutationFn: (p: Partial<UpdateTaskInput>) => updateTask(orgId, taskId, toInput(task as Task, p)),
    onSuccess: (updated) => {
      qc.setQueryData(taskKey, updated);
      qc.invalidateQueries({ queryKey: ['board', projectId] });
      qc.invalidateQueries({ queryKey: ['activity', orgId, taskId] });
    },
    onError: () => toast({ title: 'Couldn’t save changes', variant: 'error' }),
  });

  const attach = useMutation({
    mutationFn: (labelId: string) => attachLabel(orgId, taskId, labelId),
    onSuccess: () => qc.invalidateQueries({ queryKey: labelKey }),
    onError: () => toast({ title: 'Couldn’t add label', variant: 'error' }),
  });
  const detach = useMutation({
    mutationFn: (labelId: string) => detachLabel(orgId, taskId, labelId),
    onSuccess: () => qc.invalidateQueries({ queryKey: labelKey }),
    onError: () => toast({ title: 'Couldn’t remove label', variant: 'error' }),
  });

  const trash = useMutation({
    mutationFn: () => trashTask(orgId, taskId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['board', projectId] });
      toast({ title: 'Task moved to trash', variant: 'success' });
      router.push(`/app/${slug}/projects/${projectKey}`);
    },
    onError: () => toast({ title: 'Couldn’t delete task', variant: 'error' }),
  });

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading task…</p>;
  if (isError || !task) return <p className="text-sm text-destructive">Couldn’t load this task.</p>;

  const attached = taskLabels ?? [];
  const attachedIds = new Set(attached.map((l) => l.id));
  const available = (orgLabels ?? []).filter((l) => !attachedIds.has(l.id));

  return (
    <div className="grid gap-8 lg:grid-cols-[1fr_18rem]">
      {/* Main column */}
      <div className="min-w-0 space-y-6">
        <div>
          <span className="font-mono text-sm text-muted-foreground">#{task.number}</span>
          {canEdit ? (
            <input
              value={titleDraft}
              onChange={(e) => setTitleDraft(e.target.value)}
              onBlur={() => {
                const t = titleDraft.trim();
                if (t && t !== task.title) patch.mutate({ title: t });
                else setTitleDraft(task.title);
              }}
              className="mt-1 w-full rounded-xl border-0 bg-transparent text-xl font-bold tracking-tight outline-none focus:bg-secondary/50"
            />
          ) : (
            <h1 className="mt-1 text-xl font-bold tracking-tight">{task.title}</h1>
          )}
        </div>

        <div>
          <h3 className="mb-2 text-sm font-semibold">Description</h3>
          {canEdit ? (
            <div>
              <textarea
                value={descDraft}
                onChange={(e) => {
                  setDescDraft(e.target.value);
                  setDescDirty(true);
                }}
                rows={4}
                placeholder="Add a description…"
                className="w-full rounded-xl border-0 bg-secondary/60 px-4 py-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              {descDirty ? (
                <div className="mt-1 flex gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      patch.mutate({ description: descDraft });
                      setDescDirty(false);
                    }}
                    disabled={patch.isPending}
                    className="rounded bg-primary px-2 py-1 text-xs font-medium text-primary-foreground disabled:opacity-50"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setDescDraft(task.description);
                      setDescDirty(false);
                    }}
                    className="text-xs text-muted-foreground hover:text-foreground"
                  >
                    Cancel
                  </button>
                </div>
              ) : null}
            </div>
          ) : task.description ? (
            <p className="whitespace-pre-wrap text-sm">{task.description}</p>
          ) : (
            <p className="text-sm text-muted-foreground">No description.</p>
          )}
        </div>

        <Subtasks taskId={taskId} canEdit={canEdit} />
        <TaskLinks taskId={taskId} canEdit={canEdit} />
        <TaskCustomValues projectId={projectId} taskId={taskId} canEdit={canEdit} />
        <TimeTracker taskId={taskId} canEdit={canEdit} />
        <Attachments taskId={taskId} canEdit={canEdit} />
        <Comments taskId={taskId} canComment={canEdit} />
        <ActivityTimeline taskId={taskId} />
      </div>

      {/* Meta sidebar */}
      <aside className="space-y-5">
        <Field label="Assignee">
          {canEdit ? (
            <select
              value={task.assignee_id ?? ''}
              onChange={(e) => patch.mutate({ assignee_id: e.target.value || null })}
                className="w-full rounded-xl border-0 bg-secondary/60 px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Unassigned</option>
              {members.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.name || m.email}
                </option>
              ))}
            </select>
          ) : (
            <span className="text-sm">
              {members.find((m) => m.user_id === task.assignee_id)?.name ?? 'Unassigned'}
            </span>
          )}
        </Field>

        <Field label="Priority">
          {canEdit ? (
            <select
              value={task.priority}
              onChange={(e) => patch.mutate({ priority: e.target.value as Priority })}
                className="w-full rounded-xl border-0 bg-secondary/60 px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {priorityLabel(p)}
                </option>
              ))}
            </select>
          ) : (
            <span className="text-sm">{priorityLabel(task.priority as Priority)}</span>
          )}
        </Field>

        <Field label="Start date">
          {canEdit ? (
            <input
              type="date"
              value={dateInputValue(task.start_date)}
              onChange={(e) =>
                patch.mutate({ start_date: e.target.value ? new Date(e.target.value).toISOString() : null })
              }
                className="w-full rounded-xl border-0 bg-secondary/60 px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          ) : (
            <span className="text-sm">
              {task.start_date ? new Date(task.start_date).toLocaleDateString() : '—'}
            </span>
          )}
        </Field>

        <Field label="Due date">
          {canEdit ? (
            <input
              type="date"
              value={dateInputValue(task.due_date)}
              onChange={(e) =>
                patch.mutate({ due_date: e.target.value ? new Date(e.target.value).toISOString() : null })
              }
                className="w-full rounded-xl border-0 bg-secondary/60 px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          ) : (
            <span className="text-sm">
              {task.due_date ? new Date(task.due_date).toLocaleDateString() : '—'}
            </span>
          )}
        </Field>

        <Field label="Labels">
          <div className="flex flex-wrap gap-1.5">
            {attached.map((l) => (
              <span
                key={l.id}
                className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs"
                style={{ backgroundColor: `${l.color}22`, color: l.color }}
              >
                {l.name}
                {canEdit ? (
                  <button
                    type="button"
                    onClick={() => detach.mutate(l.id)}
                    aria-label={`Remove ${l.name}`}
                    className="hover:opacity-70"
                  >
                    ×
                  </button>
                ) : null}
              </span>
            ))}
            {attached.length === 0 ? <span className="text-sm text-muted-foreground">None</span> : null}
          </div>
          {canEdit && available.length > 0 ? (
            <select
              value=""
              onChange={(e) => e.target.value && attach.mutate(e.target.value)}
              className="mt-1.5 w-full rounded-xl border-0 bg-secondary/60 px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Add label…</option>
              {available.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name}
                </option>
              ))}
            </select>
          ) : null}
        </Field>

        {canEdit ? (
          <div className="border-t pt-4">
            <button
              type="button"
              onClick={() => trash.mutate()}
              disabled={trash.isPending}
              className="inline-flex items-center gap-1.5 text-sm text-destructive hover:underline disabled:opacity-50"
            >
              <Trash2 className="h-4 w-4" /> Move to trash
            </button>
          </div>
        ) : null}
      </aside>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
      {children}
    </div>
  );
}
