'use client';

import Link from 'next/link';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { CalendarDays, ListChecks, MessageSquare, Settings2 } from 'lucide-react';
import { useState, type MouseEvent as ReactMouseEvent, type SyntheticEvent } from 'react';

import type { Label, Priority, Task, TaskCard } from '@/lib/api/types';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import { attachLabel, detachLabel, getTask, updateTask } from '@/lib/api/tasks';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { Avatar } from '@/components/ui/avatar';

// Due-date presentation: overdue (red), due today (amber), upcoming (muted).
function dueState(due: string): { label: string; className: string } {
  const day = new Date(due);
  day.setHours(0, 0, 0, 0);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const label = day.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  if (day.getTime() < today.getTime())
    return { label, className: 'bg-red-500/15 text-red-600 dark:text-red-400' };
  if (day.getTime() === today.getTime())
    return { label: 'Today', className: 'bg-amber-500/15 text-amber-700 dark:text-amber-400' };
  return { label, className: 'bg-secondary text-muted-foreground' };
}

// A single kanban card. Sortable (drag handle = whole card). In select mode a
// checkbox replaces drag interaction and toggles bulk selection.
export function Card({
  task,
  href,
  selectMode,
  selected,
  onToggle,
  assigneeName,
  orgId,
  members,
  labels,
  onMutated,
}: {
  task: TaskCard;
  href: string;
  selectMode: boolean;
  selected: boolean;
  onToggle: (id: string) => void;
  assigneeName?: string;
  orgId: string;
  members: { user_id: string; name: string }[];
  labels: Label[];
  onMutated: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: task.id,
    disabled: selectMode,
  });

  const style = {
    transform: CSS.Translate.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'group rounded-2xl bg-card p-4 text-sm shadow-[0_5px_20px_hsl(var(--foreground)/0.04)] transition-all duration-150',
        isDragging && 'opacity-50 shadow-xl',
        selected && 'ring-2 ring-primary/60',
        !isDragging && !selectMode && 'hover:-translate-y-0.5 hover:shadow-[0_10px_28px_hsl(var(--foreground)/0.08)]',
      )}
      {...(selectMode ? {} : attributes)}
      {...(selectMode ? {} : listeners)}
    >
      <div className="flex items-start gap-2">
        {selectMode ? (
          <input
            type="checkbox"
            checked={selected}
            onChange={() => onToggle(task.id)}
            className="mt-0.5 h-4 w-4 rounded border-input accent-primary"
            aria-label={`Select task ${task.title}`}
          />
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-1">
            {selectMode ? (
              <p className="truncate font-medium">{task.title}</p>
            ) : (
              <Link
                href={href}
                onClick={(e) => e.stopPropagation()}
                className="truncate font-medium transition-colors hover:text-primary"
              >
                {task.title}
              </Link>
            )}
            {!selectMode ? (
              <QuickMenu
                task={task}
                orgId={orgId}
                members={members}
                labels={labels}
                onMutated={onMutated}
              />
            ) : null}
          </div>
          {task.labels.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-1">
              {task.labels.slice(0, 3).map((l) => (
                <span
                  key={l.id}
                  className="inline-flex items-center gap-1 rounded-full bg-secondary px-2 py-0.5 text-[10px] font-medium text-secondary-foreground"
                  title={l.name}
                >
                  <span
                    className="h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{ backgroundColor: l.color }}
                  />
                  <span className="max-w-24 truncate">{l.name}</span>
                </span>
              ))}
              {task.labels.length > 3 ? (
                <span className="rounded-full bg-secondary px-2 py-0.5 text-[10px] text-muted-foreground">
                  +{task.labels.length - 3}
                </span>
              ) : null}
            </div>
          ) : null}
          <div className="mt-2 flex items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">#{task.number}</span>
            {task.priority !== 'none' ? (
              <Badge
                variant={
                  task.priority === 'urgent' || task.priority === 'high'
                    ? 'destructive'
                    : task.priority === 'medium'
                      ? 'warning'
                      : 'secondary'
                }
              >
                {priorityLabel(task.priority)}
              </Badge>
            ) : null}
            {task.due_date ? (
              <span
                className={cn(
                  'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium',
                  dueState(task.due_date).className,
                )}
                title={`Due ${new Date(task.due_date).toLocaleDateString()}`}
              >
                <CalendarDays className="h-3 w-3" />
                {dueState(task.due_date).label}
              </span>
            ) : null}
            {task.assignee_id ? (
              <Avatar name={assigneeName ?? task.assignee_id} size="sm" className="ml-auto" />
            ) : null}
          </div>
          {task.comment_count > 0 || task.subtask_total > 0 ? (
            <div className="mt-2 flex items-center gap-3 text-[11px] text-muted-foreground">
              {task.comment_count > 0 ? (
                <span className="inline-flex items-center gap-1" title={`${task.comment_count} comments`}>
                  <MessageSquare className="h-3 w-3" />
                  {task.comment_count}
                </span>
              ) : null}
              {task.subtask_total > 0 ? (
                <span
                  className="inline-flex items-center gap-1.5"
                  title={`${task.subtask_done}/${task.subtask_total} subtasks done`}
                >
                  <ListChecks className="h-3 w-3" />
                  {task.subtask_done}/{task.subtask_total}
                  <span className="h-1 w-12 overflow-hidden rounded-full bg-secondary">
                    <span
                      className="block h-full rounded-full bg-primary"
                      style={{ width: `${Math.round((task.subtask_done / task.subtask_total) * 100)}%` }}
                    />
                  </span>
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

// Hover quick menu (Jira-style): change priority, assignee, or labels without
// opening the detail page. Loads the full task once for the PATCH base, then
// applies in place and refreshes the board.
function QuickMenu({
  task,
  orgId,
  members,
  labels,
  onMutated,
}: {
  task: TaskCard;
  orgId: string;
  members: { user_id: string; name: string }[];
  labels: Label[];
  onMutated: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [full, setFull] = useState<Task | null>(null);
  const [busy, setBusy] = useState(false);

  async function ensureFull(): Promise<Task | null> {
    if (full) return full;
    try {
      const t = await getTask(orgId, task.id);
      setFull(t);
      return t;
    } catch {
      return null;
    }
  }

  function openMenu(e: ReactMouseEvent) {
    e.stopPropagation();
    setOpen((v) => !v);
    if (!open) void ensureFull();
  }

  async function patch(p: { priority?: Priority; assignee_id?: string | null }) {
    const base = await ensureFull();
    if (!base) return;
    setBusy(true);
    try {
      const next = await updateTask(orgId, task.id, {
        title: base.title,
        description: base.description,
        assignee_id: p.assignee_id !== undefined ? p.assignee_id : (base.assignee_id ?? null),
        priority: (p.priority ?? base.priority) as Priority,
        due_date: base.due_date ?? null,
      });
      setFull(next);
      onMutated();
    } finally {
      setBusy(false);
    }
  }

  async function toggleLabel(labelId: string, attached: boolean) {
    setBusy(true);
    try {
      if (attached) await detachLabel(orgId, task.id, labelId);
      else await attachLabel(orgId, task.id, labelId);
      onMutated();
    } finally {
      setBusy(false);
    }
  }

  const attachedIds = new Set(task.labels.map((l) => l.id));
  const stop = (e: SyntheticEvent) => e.stopPropagation();

  return (
    <div className="relative shrink-0" onPointerDown={stop} onClick={stop}>
      <button
        type="button"
        onClick={openMenu}
        aria-label="Quick actions"
        title="Quick actions"
        className="rounded-md p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-secondary hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100"
      >
        <Settings2 className="h-3.5 w-3.5" />
      </button>
      {open ? (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} aria-hidden />
          <div className="absolute right-0 top-full z-40 mt-1 w-52 space-y-2 rounded-xl border bg-popover p-2 shadow-lg">
            <label className="block px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Priority
            </label>
            <div className="flex flex-wrap gap-1 px-1">
              {PRIORITIES.filter((p) => p !== 'none').map((p) => (
                <button
                  key={p}
                  type="button"
                  disabled={busy}
                  onClick={() => void patch({ priority: p })}
                  className={cn(
                    'rounded-full px-2 py-0.5 text-[10px] font-medium transition-colors',
                    task.priority === p
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-secondary text-secondary-foreground hover:bg-secondary/70',
                  )}
                >
                  {priorityLabel(p)}
                </button>
              ))}
            </div>
            <label className="block px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Assignee
            </label>
            <select
              value={task.assignee_id ?? ''}
              disabled={busy}
              onChange={(e) => void patch({ assignee_id: e.target.value || null })}
              className="w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none"
            >
              <option value="">Unassigned</option>
              {members.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.name}
                </option>
              ))}
            </select>
            {labels.length > 0 ? (
              <>
                <span className="block px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  Labels
                </span>
                <div className="max-h-28 space-y-0.5 overflow-y-auto px-1">
                  {labels.map((l) => (
                    <label
                      key={l.id}
                      className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-xs hover:bg-secondary"
                    >
                      <input
                        type="checkbox"
                        checked={attachedIds.has(l.id)}
                        disabled={busy}
                        onChange={() => void toggleLabel(l.id, attachedIds.has(l.id))}
                        className="h-3.5 w-3.5 rounded border-input accent-primary"
                      />
                      <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: l.color }} />
                      <span className="truncate">{l.name}</span>
                    </label>
                  ))}
                </div>
              </>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}
