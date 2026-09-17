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
    { href: `${base}/analytics`, label: 'Analytics', exact: false },
    ...(isAdmin ? [{ href: `${base}/settings`, label: 'Settings', exact: false }] : []),
  ];

  return (
    <div className="mb-6 border-b border-border pb-0">
      <div className="flex items-center gap-2">
        <span
          className="h-3 w-3 shrink-0 rounded-sm"
          style={{ backgroundColor: project.color || 'hsl(var(--muted-foreground))' }}
        />
        <h1 className="text-2xl font-semibold tracking-[-0.04em]">{project.name}</h1>
        <span className="font-mono text-[11px] uppercase tracking-wider text-muted-foreground">{project.key}</span>
        {project.archived_at ? (
          <span className="inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
            <Archive className="h-3 w-3" /> Archived
          </span>
        ) : null}
      </div>
      <nav className="mt-5 flex gap-5">
        {tabs.map((t) => {
          const active = t.exact ? pathname === t.href : pathname.startsWith(t.href);
          return (
            <Link
              key={t.href}
              href={t.href}
              className={cn(
                'border-b-2 px-0 pb-3 text-sm transition-colors',
                active
                  ? 'border-primary font-medium text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
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
