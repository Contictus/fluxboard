'use client';

import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Play, CheckCircle2, Trash2, Plus } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { getBoard } from '@/lib/api/board';
import {
  assignSprint,
  completeSprint,
  createSprint,
  deleteSprint,
  listSprints,
  startSprint,
  type Sprint,
} from '@/lib/api/sprints';
import { ProjectHeader } from '@/components/board/project-header';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Field } from '@/components/auth/field';
import { useToast } from '@/components/ui/toast';
import { cn } from '@/lib/utils';
import { ApiError } from '@/lib/api/client';

function statusBadge(s: Sprint['status']): string {
  return s === 'active'
    ? 'bg-green-500/15 text-green-700 dark:text-green-400'
    : s === 'completed'
      ? 'bg-secondary text-muted-foreground'
      : 'bg-sky-500/15 text-sky-700 dark:text-sky-400';
}

export default function ProjectSprintsPage({ params }: { params: { projectKey: string } }) {
  const { orgId, slug } = useOrg();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  const sprintsKey = ['sprints', project?.id] as const;
  const sprintsQ = useQuery({
    queryKey: sprintsKey,
    queryFn: () => listSprints(orgId, project!.id),
    enabled: Boolean(project),
  });
  const boardQ = useQuery({
    queryKey: ['board', project?.id],
    queryFn: () => getBoard(orgId, project!.id),
    enabled: Boolean(project),
  });
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: sprintsKey });
    queryClient.invalidateQueries({ queryKey: ['board', project?.id] });
  };
  const onError = (err: unknown, fallback: string) =>
    toast({
      title: err instanceof ApiError && err.status === 409 ? 'Another sprint is already active' : fallback,
      variant: 'error',
    });

  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [goal, setGoal] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');

  const createMutation = useMutation({
    mutationFn: () =>
      createSprint(orgId, project!.id, {
        name: name.trim(),
        goal: goal.trim(),
        started_at: start ? new Date(start).toISOString() : null,
        ended_at: end ? new Date(end).toISOString() : null,
      }),
    onSuccess: () => {
      invalidate();
      setShowForm(false);
      setName('');
      setGoal('');
      setStart('');
      setEnd('');
    },
    onError: (e) => onError(e, 'Could not create sprint'),
  });
  const startMutation = useMutation({
    mutationFn: (id: string) => startSprint(orgId, project!.id, id),
    onSuccess: invalidate,
    onError: (e) => onError(e, 'Could not start sprint'),
  });
  const completeMutation = useMutation({
    mutationFn: (id: string) => completeSprint(orgId, project!.id, id),
    onSuccess: invalidate,
    onError: (e) => onError(e, 'Could not complete sprint'),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSprint(orgId, project!.id, id),
    onSuccess: invalidate,
  });
  const assignMutation = useMutation({
    mutationFn: (v: { taskIds: string[]; sprintId: string | null }) =>
      assignSprint(orgId, project!.id, { task_ids: v.taskIds, sprint_id: v.sprintId }),
    onSuccess: invalidate,
    onError: (e) => onError(e, 'Could not assign tasks'),
  });

  const allTasks = useMemo(() => {
    if (!boardQ.data) return [];
    return boardQ.data.columns.flatMap((c) => c.tasks.map((t) => ({ ...t, columnName: c.name })));
  }, [boardQ.data]);
  const backlog = useMemo(() => allTasks.filter((t) => !t.sprint_id), [allTasks]);
  const tasksOf = (sprintId: string) => allTasks.filter((t) => t.sprint_id === sprintId);

  if (isLoading) return <p className="px-6 py-8 text-sm text-muted-foreground">Loading sprints…</p>;
  if (isError || !project)
    return <p className="px-6 py-8 text-sm text-destructive">Couldn&apos;t load this project.</p>;

  const sprints = sprintsQ.data ?? [];

  return (
    <div className="px-6 py-6">
      <ProjectHeader project={project} />

      <div className="mb-6 flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Plan iterations, assign work, then start the sprint. Completing snapshots velocity and
          returns unfinished tasks to the backlog.
        </p>
        <Button size="sm" onClick={() => setShowForm((v) => !v)}>
          <Plus className="h-4 w-4" /> New sprint
        </Button>
      </div>

      {showForm ? (
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>New sprint</CardTitle>
            <CardDescription>Timebox a batch of work.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="sprint-name" label="Name">
                <input
                  id="sprint-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Sprint 12"
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </Field>
              <Field id="sprint-goal" label="Goal (optional)">
                <input
                  id="sprint-goal"
                  value={goal}
                  onChange={(e) => setGoal(e.target.value)}
                  placeholder="e.g. Ship onboarding"
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </Field>
              <Field id="sprint-start" label="Start (optional)">
                <input
                  id="sprint-start"
                  type="date"
                  value={start}
                  onChange={(e) => setStart(e.target.value)}
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none"
                />
              </Field>
              <Field id="sprint-end" label="End (optional)">
                <input
                  id="sprint-end"
                  type="date"
                  value={end}
                  onChange={(e) => setEnd(e.target.value)}
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none"
                />
              </Field>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={!name.trim() || createMutation.isPending}
                onClick={() => createMutation.mutate()}
              >
                {createMutation.isPending ? 'Creating…' : 'Create sprint'}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setShowForm(false)}>
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="space-y-3">
        {sprints.length === 0 ? (
          <p className="text-sm text-muted-foreground">No sprints yet. Create the first one above.</p>
        ) : (
          sprints.map((s) => {
            const mine = tasksOf(s.id);
            return (
              <Card key={s.id}>
                <CardContent className="flex flex-wrap items-center gap-3 py-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="truncate text-sm font-semibold">{s.name}</p>
                      <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', statusBadge(s.status))}>
                        {s.status}
                      </span>
                    </div>
                    {s.goal ? <p className="truncate text-xs text-muted-foreground">{s.goal}</p> : null}
                    <p className="mt-1 text-xs text-muted-foreground">
                      {mine.length} tasks
                      {s.status === 'completed'
                        ? ` · velocity ${s.completed_done}/${s.completed_total}`
                        : ''}
                    </p>
                  </div>
                  {s.status === 'planned' ? (
                    <Button size="sm" variant="outline" onClick={() => startMutation.mutate(s.id)}>
                      <Play className="h-3.5 w-3.5" /> Start
                    </Button>
                  ) : null}
                  {s.status === 'active' ? (
                    <Button size="sm" variant="outline" onClick={() => completeMutation.mutate(s.id)}>
                      <CheckCircle2 className="h-3.5 w-3.5" /> Complete
                    </Button>
                  ) : null}
                  {s.status === 'planned' ? (
                    <button
                      type="button"
                      onClick={() => deleteMutation.mutate(s.id)}
                      aria-label={`Delete ${s.name}`}
                      className="rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  ) : null}
                </CardContent>
                {mine.length > 0 ? (
                  <div className="flex flex-wrap gap-1.5 border-t px-4 py-3">
                    {mine.map((t) => (
                      <a
                        key={t.id}
                        href={`/app/${slug}/projects/${project.key}/tasks/${t.number}`}
                        className="rounded-lg bg-secondary px-2 py-1 text-xs hover:bg-secondary/70"
                        title={t.title}
                      >
                        <span className="mr-1 font-mono text-[10px] text-muted-foreground">#{t.number}</span>
                        <span className="max-w-40 truncate align-middle">{t.title}</span>
                      </a>
                    ))}
                    {s.status !== 'completed' ? (
                      <button
                        type="button"
                        onClick={() => assignMutation.mutate({ taskIds: mine.map((t) => t.id), sprintId: null })}
                        className="rounded-lg px-2 py-1 text-xs text-muted-foreground hover:bg-secondary hover:text-foreground"
                      >
                        Move all to backlog
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </Card>
            );
          })
        )}
      </div>

      {backlog.length > 0 ? (
        <div className="mt-8">
          <h3 className="mb-2 text-sm font-semibold">Backlog ({backlog.length})</h3>
          <div className="space-y-1.5">
            {backlog.map((t) => (
              <div key={t.id} className="flex items-center gap-2 rounded-xl border bg-card px-3 py-2 text-sm">
                <span className="font-mono text-xs text-muted-foreground">#{t.number}</span>
                <span className="min-w-0 flex-1 truncate">{t.title}</span>
                <select
                  defaultValue=""
                  onChange={(e) => {
                    if (e.target.value) {
                      assignMutation.mutate({ taskIds: [t.id], sprintId: e.target.value });
                      e.target.value = '';
                    }
                  }}
                  aria-label={`Add ${t.title} to sprint`}
                  className="rounded-lg border border-input bg-background px-2 py-1 text-xs outline-none"
                >
                  <option value="">Add to sprint…</option>
                  {sprints
                    .filter((s) => s.status !== 'completed')
                    .map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                </select>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
