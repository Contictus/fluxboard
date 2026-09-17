'use client';

import { useState } from 'react';
import { useDroppable } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { Plus } from 'lucide-react';

import type { BoardColumn, Priority, TaskCard } from '@/lib/api/types';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';
import { Card } from './card';

export interface QuickTaskInput {
  title: string;
  priority: Priority;
  assignee_id: string | null;
  due_date: string | null;
}

export interface AssigneeOption {
  user_id: string;
  name: string;
}

// One board column: a droppable region wrapping a sortable list of cards, a WIP
// indicator, and an inline "add task" composer.
export function Column({
  column,
  tasks,
  cardHref,
  selectMode,
  selectedIds,
  onToggleSelect,
  onAddTask,
  addPending,
  members,
}: {
  column: BoardColumn;
  tasks: TaskCard[];
  cardHref: (task: TaskCard) => string;
  selectMode: boolean;
  selectedIds: Set<string>;
  onToggleSelect: (id: string) => void;
  onAddTask: (columnId: string, input: QuickTaskInput) => void;
  addPending: boolean;
  members: AssigneeOption[];
}) {
  const { setNodeRef, isOver } = useDroppable({ id: column.id });
  const [composing, setComposing] = useState(false);
  const [title, setTitle] = useState('');
  const [priority, setPriority] = useState<Priority>('none');
  const [assigneeId, setAssigneeId] = useState('');
  const [dueDate, setDueDate] = useState('');

  const overLimit = column.wip_limit != null && tasks.length >= column.wip_limit;

  function resetComposer() {
    setTitle('');
    setPriority('none');
    setAssigneeId('');
    setDueDate('');
    setComposing(false);
  }

  function submit() {
    const t = title.trim();
    if (!t) return;
    onAddTask(column.id, {
      title: t,
      priority,
      assignee_id: assigneeId || null,
      due_date: dueDate ? new Date(dueDate).toISOString() : null,
    });
    resetComposer();
  }

  return (
    <div className="flex w-80 shrink-0 flex-col rounded-3xl bg-secondary/45 p-2">
      <div className="flex items-center gap-2 px-3 py-3">
        <h3 className="font-display text-sm font-semibold">{column.name}</h3>
        <span
          className={cn(
            'font-mono text-[10px]',
            overLimit ? 'bg-amber-500/20 text-amber-700 dark:text-amber-400' : 'text-muted-foreground',
          )}
        >
          {tasks.length}
          {column.wip_limit != null ? ` / ${column.wip_limit}` : ''}
        </span>
      </div>

      <div
        ref={setNodeRef}
        className={cn(
          'flex min-h-[3rem] flex-1 flex-col gap-3 px-1 pb-2 transition-colors',
          isOver && 'rounded-2xl bg-primary/8',
        )}
      >
        <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
          {tasks.map((t) => (
            <Card
              key={t.id}
              task={t}
              href={cardHref(t)}
              selectMode={selectMode}
              selected={selectedIds.has(t.id)}
              onToggle={onToggleSelect}
              assigneeName={t.assignee_id ? members.find((m) => m.user_id === t.assignee_id)?.name : undefined}
            />
          ))}
        </SortableContext>

        {composing ? (
          <div className="rounded-2xl bg-card p-3 shadow-sm">
            <textarea
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  submit();
                }
                if (e.key === 'Escape') resetComposer();
              }}
              rows={2}
              placeholder="Task title…"
              className="w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            <div className="mt-2 grid grid-cols-2 gap-2">
              <select
                value={priority}
                onChange={(e) => setPriority(e.target.value as Priority)}
                aria-label="Priority"
                className="w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {PRIORITIES.map((p) => (
                  <option key={p} value={p}>
                    {priorityLabel(p)}
                  </option>
                ))}
              </select>
              <select
                value={assigneeId}
                onChange={(e) => setAssigneeId(e.target.value)}
                aria-label="Assignee"
                className="w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="">Unassigned</option>
                {members.map((m) => (
                  <option key={m.user_id} value={m.user_id}>
                    {m.name}
                  </option>
                ))}
              </select>
              <input
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
                aria-label="Due date"
                className="col-span-2 w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
            <div className="mt-2 flex items-center gap-2">
              <button
                onClick={submit}
                disabled={addPending || !title.trim()}
                className="rounded-xl bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground disabled:opacity-50"
              >
                {addPending ? 'Adding…' : 'Add'}
              </button>
              <button
                onClick={resetComposer}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <button
            onClick={() => setComposing(true)}
            className="flex items-center gap-1.5 rounded-xl px-3 py-2.5 text-left text-xs text-muted-foreground hover:bg-background hover:text-foreground"
          >
            <Plus className="h-3.5 w-3.5" /> Add task
          </button>
        )}
      </div>
    </div>
  );
}
