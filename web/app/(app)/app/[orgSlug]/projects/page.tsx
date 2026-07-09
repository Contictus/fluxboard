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
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Projects</h1>
          <p className="text-sm text-muted-foreground">Boards, tasks and workflows.</p>
        </div>
        <Link
          href={`/app/${slug}/projects/new`}
          className="inline-flex h-10 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" /> New project
        </Link>
      </div>

      <div className="mb-6 inline-flex rounded-md border bg-card p-0.5 text-sm">
        {(['active', 'archived'] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={cn(
              'rounded px-3 py-1.5 capitalize transition-colors',
              filter === f ? 'bg-secondary font-medium' : 'text-muted-foreground hover:text-foreground',
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
        <div className="rounded-lg border border-dashed p-10 text-center">
          <p className="text-sm text-muted-foreground">
            {filter === 'archived' ? 'No archived projects.' : 'No projects yet.'}
          </p>
          {filter === 'active' ? (
            <Link
              href={`/app/${slug}/projects/new`}
              className="mt-3 inline-block text-sm text-primary hover:underline"
            >
              Create your first project →
            </Link>
          ) : null}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => (
            <Link
              key={p.id}
              href={`/app/${slug}/projects/${p.key}`}
              className="group rounded-lg border bg-card p-4 transition-colors hover:border-primary/50"
            >
              <div className="flex items-center gap-2">
                <span
                  className="h-3 w-3 shrink-0 rounded-full"
                  style={{ backgroundColor: p.color || 'hsl(var(--muted-foreground))' }}
                />
                <span className="font-medium">{p.name}</span>
                {p.archived_at ? (
                  <span className="ml-auto inline-flex items-center gap-1 rounded bg-secondary px-1.5 py-0.5 text-xs text-muted-foreground">
                    <Archive className="h-3 w-3" /> Archived
                  </span>
                ) : (
                  <span className="ml-auto text-xs text-muted-foreground">{p.key}</span>
                )}
              </div>
              {p.description ? (
                <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">{p.description}</p>
              ) : null}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
