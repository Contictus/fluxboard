'use client';

import { useState } from 'react';
import { useDroppable } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { Plus } from 'lucide-react';

import type { BoardColumn, TaskCard } from '@/lib/api/types';
import { cn } from '@/lib/utils';
import { Card } from './card';

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
}: {
  column: BoardColumn;
  tasks: TaskCard[];
  cardHref: (task: TaskCard) => string;
  selectMode: boolean;
  selectedIds: Set<string>;
  onToggleSelect: (id: string) => void;
  onAddTask: (columnId: string, title: string) => void;
  addPending: boolean;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: column.id });
  const [composing, setComposing] = useState(false);
  const [title, setTitle] = useState('');

  const overLimit = column.wip_limit != null && tasks.length >= column.wip_limit;

  function submit() {
    const t = title.trim();
    if (!t) return;
    onAddTask(column.id, t);
    setTitle('');
    setComposing(false);
  }

  return (
    <div className="flex w-72 shrink-0 flex-col border-t-2 border-border bg-secondary/35">
      <div className="flex items-center gap-2 px-3 py-3">
        <h3 className="font-mono text-[11px] font-semibold uppercase tracking-[0.14em]">{column.name}</h3>
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
          'flex min-h-[3rem] flex-1 flex-col gap-2 px-2 pb-3 transition-colors',
          isOver && 'bg-primary/5',
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
            />
          ))}
        </SortableContext>

        {composing ? (
          <div className="rounded-sm border border-border bg-card p-2">
            <textarea
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  submit();
                }
                if (e.key === 'Escape') setComposing(false);
              }}
              rows={2}
              placeholder="Task title…"
              className="w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            <div className="mt-1 flex items-center gap-2">
              <button
                onClick={submit}
                disabled={addPending || !title.trim()}
                className="rounded-sm bg-primary px-2 py-1 text-xs font-medium text-primary-foreground disabled:opacity-50"
              >
                {addPending ? 'Adding…' : 'Add'}
              </button>
              <button
                onClick={() => setComposing(false)}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <button
            onClick={() => setComposing(true)}
            className="flex items-center gap-1.5 rounded-sm px-2 py-2 text-left text-xs text-muted-foreground hover:bg-secondary hover:text-foreground"
          >
            <Plus className="h-3.5 w-3.5" /> Add task
          </button>
        )}
      </div>
    </div>
  );
}
