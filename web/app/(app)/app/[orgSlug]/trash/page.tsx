'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { RotateCcw } from 'lucide-react';

import { listTrash, restoreTask } from '@/lib/api/tasks';
import { useOrg } from '@/lib/org/context';
import { useProjectsMap } from '@/lib/board/use-projects';
import { useToast } from '@/components/ui/toast';
import { TaskRow } from '@/components/task/task-row';
import type { Task } from '@/lib/api/types';

// Org-wide trash (7.6.5, FR-TASK-009). The API exposes trash per project, so the
// org view fans out ListTrash across every project and flattens (ADR-018). There
// is no hard-purge endpoint — trash auto-purges via a retention job — so this is
// restore-only; purge is surfaced as an explanatory note, not an action.
export default function TrashPage() {
  const { orgId, slug, role } = useOrg();
  const { projects, byId, isLoading: projectsLoading } = useProjectsMap();
  const { toast } = useToast();
  const qc = useQueryClient();
  const canRestore = role !== 'GUEST';

  const projectIds = projects.map((p) => p.id);
  const trashKey = ['trash', orgId, projectIds] as const;

  const { data, isLoading, isError } = useQuery({
    queryKey: trashKey,
    queryFn: async () => {
      const perProject = await Promise.all(projectIds.map((id) => listTrash(orgId, id)));
      return perProject.flat();
    },
    enabled: !projectsLoading,
  });

  const restore = useMutation({
    mutationFn: (taskId: string) => restoreTask(orgId, taskId),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: trashKey });
      qc.invalidateQueries({ queryKey: ['board', t.project_id] });
      toast({ title: 'Task restored', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t restore task', variant: 'error' }),
  });

  const tasks: Task[] = data ?? [];

  return (
    <div className="px-6 py-6">
      <h1 className="text-xl font-bold tracking-tight">Trash</h1>
      <p className="mb-4 mt-1 text-sm text-muted-foreground">
        Deleted tasks are kept temporarily and purged automatically. Restore anything you still need.
      </p>

      {projectsLoading || isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : isError ? (
        <p className="text-sm text-destructive">Couldn’t load trash.</p>
      ) : tasks.length === 0 ? (
        <p className="text-sm text-muted-foreground">Trash is empty.</p>
      ) : (
        <div className="space-y-2">
          {tasks.map((t) => (
            <TaskRow
              key={t.id}
              task={t}
              project={byId.get(t.project_id)}
              orgSlug={slug}
              disableLink
              action={
                canRestore ? (
                  <button
                    type="button"
                    onClick={() => restore.mutate(t.id)}
                    disabled={restore.isPending}
                    className="inline-flex shrink-0 items-center gap-1 rounded-md border px-2 py-1 text-xs hover:bg-secondary disabled:opacity-50"
                  >
                    <RotateCcw className="h-3.5 w-3.5" /> Restore
                  </button>
                ) : null
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}
