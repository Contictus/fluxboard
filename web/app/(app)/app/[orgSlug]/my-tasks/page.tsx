'use client';

import { useQuery } from '@tanstack/react-query';

import { searchTasks } from '@/lib/api/tasks';
import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { useProjectsMap } from '@/lib/board/use-projects';
import { TaskRow } from '@/components/task/task-row';
import type { Task } from '@/lib/api/types';

// Cross-project "assigned to me", grouped by project (7.6.5). There is no server
// assignee=me alias, so the caller's own id is passed to search (ADR-016).
export default function MyTasksPage() {
  const { orgId, slug } = useOrg();
  const { userId } = useAuth();
  const { byId } = useProjectsMap();

  const { data, isLoading, isError } = useQuery({
    queryKey: ['my-tasks', orgId, userId],
    queryFn: () => searchTasks(orgId, { assignee_id: userId as string, limit: 100 }),
    enabled: Boolean(userId),
  });

  const tasks = data?.tasks ?? [];

  // Group by project_id, preserving first-seen order.
  const groups: { projectId: string; tasks: Task[] }[] = [];
  const index = new Map<string, number>();
  for (const t of tasks) {
    let i = index.get(t.project_id);
    if (i === undefined) {
      i = groups.length;
      index.set(t.project_id, i);
      groups.push({ projectId: t.project_id, tasks: [] });
    }
    groups[i]!.tasks.push(t);
  }

  return (
    <div className="px-6 py-6">
      <h1 className="mb-4 text-xl font-bold tracking-tight">My tasks</h1>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : isError ? (
        <p className="text-sm text-destructive">Couldn’t load your tasks.</p>
      ) : tasks.length === 0 ? (
        <p className="text-sm text-muted-foreground">Nothing assigned to you right now.</p>
      ) : (
        <div className="space-y-6">
          {groups.map((g) => {
            const project = byId.get(g.projectId);
            return (
              <section key={g.projectId}>
                <h2 className="mb-2 text-sm font-semibold text-muted-foreground">
                  {project ? project.name : 'Unknown project'}
                </h2>
                <div className="space-y-2">
                  {g.tasks.map((t) => (
                    <TaskRow key={t.id} task={t} project={project} orgSlug={slug} />
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}
