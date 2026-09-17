'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';

// Org-settings shell (docs/02 §7). The whole area is ADMIN+ (OWNER-only controls
// are further gated inside General/Danger). A non-admin sees a read-only notice
// instead of the tabs — mirrors the project-settings gate.
export default function SettingsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { slug, isAdmin } = useOrg();
  const base = `/app/${slug}/settings`;

  const nav = [
    { href: base, label: 'General', exact: true },
    { href: `${base}/members`, label: 'Members' },
    { href: `${base}/labels`, label: 'Labels' },
    { href: `${base}/automations`, label: 'Automations' },
    { href: `${base}/api-keys`, label: 'API keys' },
    { href: `${base}/audit-log`, label: 'Audit log' },
    { href: `${base}/danger`, label: 'Danger zone' },
  ];

  return (
    <div className="mx-auto max-w-6xl px-6 py-12 lg:px-10 lg:py-16">
      <p className="mb-3 text-xs font-semibold uppercase tracking-[0.16em] text-primary">Workspace / settings</p>
      <h1 className="mb-10 text-4xl font-semibold tracking-[-0.06em]">Organization settings</h1>
      {isAdmin ? (
        <div className="grid gap-8 md:grid-cols-[180px_1fr]">
          <nav className="flex flex-row gap-1 overflow-x-auto md:flex-col md:pr-8">
            {nav.map((n) => {
              const active = n.exact ? pathname === n.href : pathname.startsWith(n.href);
              return (
                <Link
                  key={n.href}
                  href={n.href}
                  className={cn(
                    'whitespace-nowrap rounded-xl px-3 py-2.5 text-sm transition-colors',
                    active
                      ? 'bg-secondary font-medium text-foreground'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
                  )}
                >
                  {n.label}
                </Link>
              );
            })}
          </nav>
          <div>{children}</div>
        </div>
      ) : (
        <p className="max-w-2xl rounded-2xl bg-secondary/55 p-5 text-sm text-muted-foreground">
          Only organization owners and admins can change organization settings.
        </p>
      )}
    </div>
  );
}
