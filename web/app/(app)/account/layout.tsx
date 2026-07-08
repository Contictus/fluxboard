'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';

import { cn } from '@/lib/utils';

const nav = [
  { href: '/account/profile', label: 'Profile' },
  { href: '/account/security', label: 'Security' },
  { href: '/account/sessions', label: 'Sessions' },
  { href: '/account/notifications', label: 'Notifications' },
  { href: '/account/danger', label: 'Danger zone' },
];

export default function AccountLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="mx-auto max-w-4xl px-6 py-10">
      <div className="mb-8 flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">Account</h1>
        <div className="flex items-center gap-4 text-sm">
          <Link href="/app" className="text-muted-foreground hover:text-foreground">
            Organizations
          </Link>
          <Link href="/logout" className="text-muted-foreground hover:text-foreground">
            Sign out
          </Link>
        </div>
      </div>
      <div className="grid gap-8 md:grid-cols-[180px_1fr]">
        <nav className="flex flex-row gap-1 overflow-x-auto md:flex-col">
          {nav.map((n) => {
            const active = pathname === n.href;
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
    </div>
  );
}
