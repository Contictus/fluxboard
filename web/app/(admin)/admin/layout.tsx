'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Shield, Building2, ScrollText, ListChecks, LayoutDashboard, ExternalLink } from 'lucide-react';

import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth/context';
import { getMe } from '@/lib/api/user';

// Platform-admin shell (docs/02 §10). A SEPARATE route group with its own dark
// chrome, deliberately outside the org shell + tenant context. Gate: the caller's
// platform_role must be `admin` (the backend additionally enforces 2FA on every
// /admin call). Non-admins are bounced to /app.
export default function AdminLayout({ children }: { children: ReactNode }) {
  const { loading, isAuthenticated } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  const me = useQuery({ queryKey: ['me'], queryFn: getMe, enabled: isAuthenticated });
  const isPlatformAdmin = me.data?.platform_role === 'admin';

  useEffect(() => {
    if (loading) return;
    if (!isAuthenticated) {
      router.replace('/login?next=/admin');
      return;
    }
    if (me.isSuccess && !isPlatformAdmin) router.replace('/app');
  }, [loading, isAuthenticated, me.isSuccess, isPlatformAdmin, router]);

  if (loading || me.isLoading || !isPlatformAdmin) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-neutral-950 text-neutral-400">
        <p className="text-sm">Checking platform access…</p>
      </div>
    );
  }

  const nav = [
    { href: '/admin', label: 'Dashboard', icon: LayoutDashboard, exact: true },
    { href: '/admin/tenants', label: 'Tenants', icon: Building2 },
    { href: '/admin/audit-log', label: 'Audit log', icon: ScrollText },
    { href: '/admin/jobs', label: 'Jobs', icon: ListChecks },
  ];

  return (
    <div className="flex min-h-screen bg-neutral-950 text-neutral-100">
      <aside className="hidden w-56 shrink-0 border-r border-neutral-800 md:block">
        <div className="flex h-14 items-center gap-2 border-b border-neutral-800 px-4">
          <Shield className="h-4 w-4 text-amber-400" />
          <span className="text-sm font-semibold">Platform admin</span>
        </div>
        <nav className="flex flex-col gap-1 p-3">
          {nav.map((it) => {
            const active = it.exact ? pathname === it.href : pathname.startsWith(it.href);
            const Icon = it.icon;
            return (
              <Link
                key={it.href}
                href={it.href}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                  active
                    ? 'bg-neutral-800 font-medium text-white'
                    : 'text-neutral-400 hover:bg-neutral-900 hover:text-white',
                )}
              >
                <Icon className="h-4 w-4" />
                {it.label}
              </Link>
            );
          })}
          <div className="my-2 border-t border-neutral-800" />
          <Link
            href="/app"
            className="flex items-center gap-2 rounded-md px-3 py-2 text-sm text-neutral-400 hover:bg-neutral-900 hover:text-white"
          >
            <ExternalLink className="h-4 w-4" /> Back to app
          </Link>
        </nav>
      </aside>
      <main className="min-w-0 flex-1 px-6 py-8">{children}</main>
    </div>
  );
}
