'use client';

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { getBoard } from '@/lib/api/board';
import { ProjectHeader } from '@/components/board/project-header';
import { Timeline } from '@/components/board/timeline';

export default function ProjectTimelinePage({ params }: { params: { projectKey: string } }) {
  const { orgId, slug } = useOrg();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  const board = useQuery({
    queryKey: ['board', project?.id],
    queryFn: () => getBoard(orgId, project!.id),
    enabled: Boolean(project),
  });

  const tasks = useMemo(() => {
    if (!board.data) return [];
    const out: { task: (typeof board.data.columns)[number]['tasks'][number]; columnName: string }[] = [];
    for (const col of board.data.columns) {
      for (const t of col.tasks) out.push({ task: t, columnName: col.name });
    }
    return out;
  }, [board.data]);

  if (isLoading || board.isLoading) {
    return <p className="px-6 py-8 text-sm text-muted-foreground">Loading timeline…</p>;
  }
  if (isError || !project) {
    return <p className="px-6 py-8 text-sm text-destructive">Couldn&apos;t load this project.</p>;
  }

  return (
    <div className="px-6 py-6">
      <ProjectHeader project={project} />
      <Timeline
        tasks={tasks}
        taskHref={(t) => `/app/${slug}/projects/${project.key}/tasks/${t.number}`}
      />
    </div>
  );
}
