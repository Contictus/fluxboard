'use client';

import Link from 'next/link';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { CalendarDays } from 'lucide-react';

import type { TaskCard } from '@/lib/api/types';
import { priorityLabel } from '@/lib/board/priority';
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
}: {
  task: TaskCard;
  href: string;
  selectMode: boolean;
  selected: boolean;
  onToggle: (id: string) => void;
  assigneeName?: string;
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
        'rounded-2xl bg-card p-4 text-sm shadow-[0_5px_20px_hsl(var(--foreground)/0.04)] transition-all duration-150',
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
        </div>
      </div>
    </div>
  );
}
