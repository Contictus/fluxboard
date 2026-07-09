'use client';

import Link from 'next/link';
import { ArrowLeft } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { useTaskIdByNumber } from '@/lib/board/use-task';
import { TaskDetail } from '@/components/task/task-detail';

// Task detail (7.6.1). The sitemap calls for an intercepted modal over the board
// with a full page on direct load; this ships the full page for both (the card
// links here). The modal-over-board interception is deferred (ADR-018) — a
// parallel/intercepting route over the client board adds fragility for little
// gain now, and the full page is the source of truth either way.
export default function TaskDetailPage({
  params,
}: {
  params: { projectKey: string; taskNumber: string };
}) {
  const { slug } = useOrg();
  const number = Number(params.taskNumber);
  const { project, isLoading: projectLoading, isError: projectError } = useProjectByKey(params.projectKey);
  const { taskId, isLoading: taskLoading, isError: taskError } = useTaskIdByNumber(project?.id, number);

  const backHref = `/app/${slug}/projects/${params.projectKey}`;

  if (Number.isNaN(number)) {
    return <NotFound backHref={backHref} label="Invalid task number." />;
  }
  if (projectLoading || (project && taskLoading)) {
    return <p className="px-6 py-8 text-sm text-muted-foreground">Loading task…</p>;
  }
  if (projectError || taskError) {
    return <p className="px-6 py-8 text-sm text-destructive">Couldn’t load this task.</p>;
  }
  if (!project) {
    return <NotFound backHref={`/app/${slug}/projects`} label="Project not found." />;
  }
  if (!taskId) {
    return <NotFound backHref={backHref} label={`No task #${number} in ${project.key}.`} />;
  }

  return (
    <div className="px-6 py-6">
      <Link
        href={backHref}
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" /> {project.name}
      </Link>
      <TaskDetail taskId={taskId} projectId={project.id} projectKey={project.key} />
    </div>
  );
}

function NotFound({ backHref, label }: { backHref: string; label: string }) {
  return (
    <div className="mx-auto max-w-md px-6 py-16 text-center">
      <h1 className="text-lg font-semibold">Not found</h1>
      <p className="mt-1 text-sm text-muted-foreground">{label}</p>
      <Link href={backHref} className="mt-4 inline-block text-sm text-primary hover:underline">
        Go back
      </Link>
    </div>
  );
}
