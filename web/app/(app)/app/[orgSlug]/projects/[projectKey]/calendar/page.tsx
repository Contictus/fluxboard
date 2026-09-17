'use client';

import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { createTask, getBoard } from '@/lib/api/board';
import { ProjectHeader } from '@/components/board/project-header';
import { TaskCalendar } from '@/components/board/calendar';
import { Button } from '@/components/ui/button';

export default function ProjectCalendarPage({ params }: { params: { projectKey: string } }) {
  const { orgId, slug } = useOrg();
  const queryClient = useQueryClient();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  const board = useQuery({
    queryKey: ['board', project?.id],
    queryFn: () => getBoard(orgId, project!.id),
    enabled: Boolean(project),
  });

  const [pendingDue, setPendingDue] = useState<string | null>(null);
  const [title, setTitle] = useState('');

  const tasks = useMemo(() => {
    if (!board.data) return [];
    return board.data.columns.flatMap((c) => c.tasks);
  }, [board.data]);

  const createMutation = useMutation({
    mutationFn: (input: { title: string; due: string }) => {
      const firstColumn = board.data!.columns[0];
      if (!firstColumn) throw new Error('No columns yet — add one on the board first.');
      return createTask(orgId, project!.id, {
        column_id: firstColumn.id,
        title: input.title,
        description: '',
        priority: 'none',
        assignee_id: null,
        due_date: input.due,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['board', project?.id] });
      setPendingDue(null);
      setTitle('');
    },
  });

  if (isLoading || board.isLoading) {
    return <p className="px-6 py-8 text-sm text-muted-foreground">Loading calendar…</p>;
  }
  if (isError || !project) {
    return <p className="px-6 py-8 text-sm text-destructive">Couldn&apos;t load this project.</p>;
  }

  return (
    <div className="px-6 py-6">
      <ProjectHeader project={project} />
      {pendingDue ? (
        <div className="mb-4 flex items-center gap-2 rounded-2xl border bg-card p-3">
          <span className="text-sm text-muted-foreground">
            New task due {new Date(pendingDue).toLocaleDateString()}:
          </span>
          <input
            autoFocus
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && title.trim()) createMutation.mutate({ title: title.trim(), due: pendingDue });
              if (e.key === 'Escape') setPendingDue(null);
            }}
            placeholder="Task title…"
            className="min-w-0 flex-1 rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <Button
            size="sm"
            disabled={!title.trim() || createMutation.isPending}
            onClick={() => title.trim() && createMutation.mutate({ title: title.trim(), due: pendingDue })}
          >
            {createMutation.isPending ? 'Adding…' : 'Add'}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setPendingDue(null)}>
            Cancel
          </Button>
        </div>
      ) : null}
      <TaskCalendar
        tasks={tasks}
        taskHref={(t) => `/app/${slug}/projects/${project.key}/tasks/${t.number}`}
        onQuickAdd={(dueISO) => setPendingDue(dueISO)}
      />
    </div>
  );
}
