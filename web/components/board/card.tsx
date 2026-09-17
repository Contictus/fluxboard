'use client';

import Link from 'next/link';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

import type { TaskCard } from '@/lib/api/types';
import { priorityLabel } from '@/lib/board/priority';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { Avatar } from '@/components/ui/avatar';

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
        'rounded-lg border bg-card p-3 text-sm shadow-sm transition-all duration-200',
        isDragging && 'opacity-50 shadow-xl scale-105',
        selected && 'ring-2 ring-primary',
        !isDragging && !selectMode && 'hover:shadow-md hover:-translate-y-0.5',
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
              className="truncate font-medium hover:text-primary transition-colors"
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
            {task.assignee_id ? (
              <Avatar name={task.assignee_id} size="sm" className="ml-auto" />
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}
