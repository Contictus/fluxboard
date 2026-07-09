'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';

import type { Priority, Project, Task } from '@/lib/api/types';
import { priorityClass, priorityLabel } from '@/lib/board/priority';
import { memberName, useOrgMembers } from '@/lib/org/use-members';
import { cn } from '@/lib/utils';

// A single task row for the org-level task views (search / my-tasks / trash).
// Links to the task detail when the project key is known; `action` renders a
// trailing control (e.g. Restore in trash).
export function TaskRow({
  task,
  project,
  orgSlug,
  action,
  disableLink,
}: {
  task: Task;
  project?: Project;
  orgSlug: string;
  action?: ReactNode;
  /** Trashed tasks aren't on the board, so their detail route can't resolve. */
  disableLink?: boolean;
}) {
  const { byId } = useOrgMembers();
  const href =
    project && !disableLink
      ? `/app/${orgSlug}/projects/${project.key}/tasks/${task.number}`
      : undefined;

  const title = (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        {project ? (
          <span className="font-mono text-xs text-muted-foreground">
            {project.key}-{task.number}
          </span>
        ) : (
          <span className="font-mono text-xs text-muted-foreground">#{task.number}</span>
        )}
        {task.priority !== 'none' ? (
          <span
            className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium', priorityClass(task.priority as Priority))}
          >
            {priorityLabel(task.priority as Priority)}
          </span>
        ) : null}
      </div>
      <p className="mt-0.5 truncate text-sm font-medium">{task.title}</p>
    </div>
  );

  return (
    <div className="flex items-center gap-3 rounded-md border bg-card px-3 py-2">
      {href ? (
        <Link href={href} className="min-w-0 flex-1 hover:underline">
          {title}
        </Link>
      ) : (
        <div className="min-w-0 flex-1">{title}</div>
      )}
      {task.assignee_id ? (
        <span className="shrink-0 text-xs text-muted-foreground">{memberName(byId, task.assignee_id)}</span>
      ) : null}
      {action}
    </div>
  );
}
