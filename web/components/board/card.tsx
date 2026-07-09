'use client';

import Link from 'next/link';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

import type { TaskCard } from '@/lib/api/types';
import { priorityClass, priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';

// A single kanban card. Sortable (drag handle = whole card). In select mode a
// checkbox replaces drag interaction and toggles bulk selection.
export function Card({
  task,
  href,
  selectMode,
  selected,
  onToggle,
}: {
  task: TaskCard;
  href: string;
  selectMode: boolean;
  selected: boolean;
  onToggle: (id: string) => void;
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
        'rounded-md border bg-card p-2.5 text-sm shadow-sm',
        isDragging && 'opacity-40',
        selected && 'ring-2 ring-primary',
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
            className="mt-0.5 h-4 w-4"
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
              className="truncate font-medium hover:underline"
            >
              {task.title}
            </Link>
          )}
          <div className="mt-1.5 flex items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">#{task.number}</span>
            {task.priority !== 'none' ? (
              <span
                className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium', priorityClass(task.priority))}
              >
                {priorityLabel(task.priority)}
              </span>
            ) : null}
            {task.assignee_id ? (
              <span
                className="ml-auto h-5 w-5 shrink-0 rounded-full bg-secondary"
                title="Assigned"
                aria-label="Assigned"
              />
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}
