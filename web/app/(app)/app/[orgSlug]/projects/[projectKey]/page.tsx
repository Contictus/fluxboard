'use client';

import Link from 'next/link';

import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { ProjectHeader } from '@/components/board/project-header';
import { Board } from '@/components/board/board';

export default function ProjectBoardPage({ params }: { params: { projectKey: string } }) {
  const { slug } = useOrg();
  const { project, isLoading, isError } = useProjectByKey(params.projectKey);

  if (isLoading) {
    return <p className="px-6 py-8 text-sm text-muted-foreground">Loading project…</p>;
  }
  if (isError) {
    return <p className="px-6 py-8 text-sm text-destructive">Couldn’t load this project.</p>;
  }
  if (!project) {
    return (
      <div className="mx-auto max-w-md px-6 py-16 text-center">
        <h1 className="text-lg font-semibold">Project not found</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          No project with key <span className="font-mono">{params.projectKey}</span> in this
          organization.
        </p>
        <Link href={`/app/${slug}/projects`} className="mt-4 inline-block text-sm text-primary hover:underline">
          Back to projects
        </Link>
      </div>
    );
  }

  return (
    <div className="px-6 py-6">
      <ProjectHeader project={project} />
      <Board projectId={project.id} projectKey={project.key} />
    </div>
  );
}
