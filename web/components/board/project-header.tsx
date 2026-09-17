'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Archive } from 'lucide-react';

import type { Project } from '@/lib/api/types';
import { useOrg } from '@/lib/org/context';
import { cn } from '@/lib/utils';

// Shared header for the project board / list / settings pages: title, key,
// archived badge and the tab nav.
export function ProjectHeader({ project }: { project: Project }) {
  const { slug, isAdmin } = useOrg();
  const pathname = usePathname();
  const base = `/app/${slug}/projects/${project.key}`;

  const tabs = [
    { href: base, label: 'Board', exact: true },
    { href: `${base}/list`, label: 'List', exact: false },
    { href: `${base}/timeline`, label: 'Timeline', exact: false },
    { href: `${base}/calendar`, label: 'Calendar', exact: false },
    { href: `${base}/sprints`, label: 'Sprints', exact: false },
    { href: `${base}/analytics`, label: 'Analytics', exact: false },
    ...(isAdmin ? [{ href: `${base}/settings`, label: 'Settings', exact: false }] : []),
  ];

  return (
    <div className="mb-8 pb-0">
      <div className="flex items-center gap-2">
        <span
          className="h-3.5 w-3.5 shrink-0 rounded-full shadow-sm"
          style={{ backgroundColor: project.color || 'hsl(var(--muted-foreground))' }}
        />
        <h1 className="text-3xl font-semibold tracking-[-0.05em]">{project.name}</h1>
        <span className="font-mono text-[11px] uppercase tracking-wider text-muted-foreground">{project.key}</span>
        {project.archived_at ? (
          <span className="inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            <Archive className="h-3 w-3" /> Archived
          </span>
        ) : null}
      </div>
      <nav className="mt-6 flex gap-2">
        {tabs.map((t) => {
          const active = t.exact ? pathname === t.href : pathname.startsWith(t.href);
          return (
            <Link
              key={t.href}
              href={t.href}
              className={cn(
                'rounded-xl px-3 py-2 text-sm transition-colors',
                active
                  ? 'bg-card font-medium text-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-secondary hover:text-foreground',
              )}
            >
              {t.label}
            </Link>
          );
        })}
      </nav>
    </div>
  );
}
