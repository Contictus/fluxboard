'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import { ChevronRight } from 'lucide-react';

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

  // Derive current section label for breadcrumb
  const currentLabel = nav.find((n) => pathname === n.href)?.label ?? 'Account';

  return (
    <div className="mx-auto max-w-4xl px-6 py-10 animate-fade-in">
      {/* Header with breadcrumb */}
      <div className="mb-8 flex items-center justify-between">
        <div>
          <div className="mb-1 flex items-center gap-1 text-sm text-muted-foreground">
            <Link href="/app" className="hover:text-foreground transition-colors">
              Organizations
            </Link>
            <ChevronRight className="h-3 w-3" />
            <span className="text-foreground font-medium">Account</span>
          </div>
          <h1 className="text-2xl font-bold tracking-tight">{currentLabel}</h1>
        </div>
        <Link
          href="/logout"
          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          Sign out
        </Link>
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
                  'relative whitespace-nowrap rounded-md px-3 py-2 text-sm transition-all duration-200',
                  active
                    ? 'bg-secondary font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
                )}
              >
                {/* Active indicator */}
                {active ? (
                  <span className="absolute left-0 top-1/2 hidden h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary md:block" />
                ) : null}
                {n.label}
              </Link>
            );
          })}
        </nav>
        <div className="animate-fade-in">{children}</div>
      </div>
    </div>
  );
}
