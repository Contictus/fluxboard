'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { Plus, Archive } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { listProjects } from '@/lib/api/projects';
import { cn } from '@/lib/utils';

type Filter = 'active' | 'archived';

export default function ProjectsPage() {
  const { orgId, slug } = useOrg();
  const [filter, setFilter] = useState<Filter>('active');

  const { data, isLoading, isError } = useQuery({
    queryKey: ['projects', orgId, filter],
    queryFn: () => listProjects(orgId, { archived: filter === 'archived' }),
  });

  // The archived list includes active projects too; when viewing "archived",
  // keep only the ones actually archived.
  const projects =
    filter === 'archived' ? (data ?? []).filter((p) => p.archived_at) : (data ?? []);

  return (
    <div className="mx-auto max-w-[1320px] px-6 py-10 lg:px-10">
      <div className="mb-8 flex items-end justify-between gap-4 border-b border-border pb-6">
        <div>
          <p className="mb-3 font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-primary">Workspace / projects</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em]">Projects</h1>
          <p className="mt-2 text-sm text-muted-foreground">Boards, tasks and workflows in one place.</p>
        </div>
        <Link
          href={`/app/${slug}/projects/new`}
          className="inline-flex h-10 items-center gap-2 rounded-sm bg-primary px-4 font-mono text-[11px] font-medium uppercase tracking-wider text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" /> New project
        </Link>
      </div>

      <div className="mb-8 inline-flex border-b border-border text-sm">
        {(['active', 'archived'] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={cn(
              'border-b-2 px-3 py-2 capitalize transition-colors',
              filter === f ? 'border-primary font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {f}
          </button>
        ))}
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : isError ? (
        <p className="text-sm text-destructive">Couldn&apos;t load projects.</p>
      ) : projects.length === 0 ? (
        <div className="border-y border-dashed border-border p-14 text-center">
          <p className="text-sm text-muted-foreground">
            {filter === 'archived' ? 'No archived projects.' : 'No projects yet.'}
          </p>
          {filter === 'active' ? (
            <Link
              href={`/app/${slug}/projects/new`}
              className="mt-3 inline-block font-mono text-xs uppercase tracking-wider text-primary hover:text-foreground"
            >
              Create your first project →
            </Link>
          ) : null}
        </div>
      ) : (
        <div className="divide-y border-y border-border">
          {projects.map((p) => (
            <Link
              key={p.id}
              href={`/app/${slug}/projects/${p.key}`}
              className="group flex items-center justify-between gap-6 bg-transparent px-0 py-5 transition-colors hover:bg-secondary/40 sm:px-3"
            >
              <div className="flex min-w-0 items-center gap-3">
                <span
                  className="h-2.5 w-2.5 shrink-0 rounded-sm"
                  style={{ backgroundColor: p.color || 'hsl(var(--muted-foreground))' }}
                />
                <span className="truncate font-medium">{p.name}</span>
                {p.archived_at ? (
                  <span className="ml-auto inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                    <Archive className="h-3 w-3" /> Archived
                  </span>
                ) : (
                  <span className="ml-auto font-mono text-[11px] text-muted-foreground">{p.key}</span>
                )}
              </div>
              {p.description ? (
                <p className="max-w-md truncate text-sm text-muted-foreground">{p.description}</p>
              ) : null}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
