'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';

// Billing shell (docs/02 §9). The whole area is ADMIN+ server-side (read:billing);
// a non-admin sees a notice instead of the tabs (mirrors the settings gate).
export default function BillingLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { slug, isAdmin } = useOrg();
  const base = `/app/${slug}/billing`;

  const nav = [
    { href: base, label: 'Overview', exact: true },
    { href: `${base}/plans`, label: 'Plans' },
    { href: `${base}/usage`, label: 'Usage' },
    { href: `${base}/invoices`, label: 'Invoices' },
  ];

  return (
    <div className="mx-auto max-w-5xl px-6 py-10">
      <h1 className="mb-8 text-2xl font-bold tracking-tight">Billing</h1>
      {isAdmin ? (
        <div className="grid gap-8 md:grid-cols-[180px_1fr]">
          <nav className="flex flex-row gap-1 overflow-x-auto md:flex-col">
            {nav.map((n) => {
              const active = n.exact ? pathname === n.href : pathname.startsWith(n.href);
              return (
                <Link
                  key={n.href}
                  href={n.href}
                  className={cn(
                    'whitespace-nowrap rounded-md px-3 py-2 text-sm transition-colors',
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
        <p className="max-w-2xl rounded-md border bg-secondary/40 p-4 text-sm text-muted-foreground">
          Only organization owners and admins can view billing.
        </p>
      )}
    </div>
  );
}
