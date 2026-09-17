'use client';

import Link from 'next/link';
import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import {
  Building2,
  Bell,
  ChevronsUpDown,
  Plus,
  User,
  LogOut,
  Check,
  Shield,
  Menu,
  ChevronRight,
  Search,
} from 'lucide-react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';
import { listMyOrgs } from '@/lib/api/orgs';
import { getUnreadCount } from '@/lib/api/notifications';
import { getMe } from '@/lib/api/user';
import { Avatar } from '@/components/ui/avatar';

export function Topbar({ onMobileMenuToggle }: { onMobileMenuToggle?: () => void }) {
  const { org, slug, role } = useOrg();

  return (
    <header className="flex h-20 shrink-0 items-center gap-3 bg-topbar px-6 lg:px-8">
      {/* Mobile hamburger */}
      {onMobileMenuToggle ? (
        <button
          type="button"
          onClick={onMobileMenuToggle}
          className="flex h-9 w-9 items-center justify-center rounded-sm text-muted-foreground hover:bg-secondary md:hidden"
          aria-label="Open sidebar"
        >
          <Menu className="h-5 w-5" />
        </button>
      ) : null}

      <OrgSwitcher currentSlug={slug} currentName={org.name} logoUrl={org.logo_url} role={role} />

      {/* Breadcrumb */}
      <Breadcrumb slug={slug} />

      <div className="ml-auto flex items-center gap-1">
        {/* Global search hint */}
        <button
          type="button"
          className="hidden items-center gap-3 rounded-xl bg-secondary/55 px-3 py-2 text-xs text-muted-foreground transition-colors hover:bg-secondary sm:flex"
          aria-label="Search"
        >
          <Search className="h-3 w-3" />
          <span>Search…</span>
          <kbd className="rounded-md bg-background/70 px-1.5 py-0.5 font-mono text-[10px] font-medium">⌘K</kbd>
        </button>
        <NotificationBell slug={slug} />
        <AccountMenu />
      </div>
    </header>
  );
}

/** Breadcrumb derived from pathname */
function Breadcrumb({ slug }: { slug: string }) {
  const pathname = usePathname();
  const base = `/app/${slug}`;
  const relative = pathname.replace(base, '').replace(/^\//, '');
  if (!relative) return null;

  const parts = relative.split('/').filter(Boolean);
  // Capitalize first letter
  const labels = parts.map((p) => p.charAt(0).toUpperCase() + p.slice(1).replace(/-/g, ' '));

  return (
    <div className="hidden items-center gap-1 text-sm text-muted-foreground md:flex">
      {labels.map((label, i) => (
        <span key={i} className="flex items-center gap-1">
          <ChevronRight className="h-3 w-3" />
          <span className={cn(i === labels.length - 1 && 'text-foreground font-medium')}>
            {label}
          </span>
        </span>
      ))}
    </div>
  );
}

/** Lightweight click-outside dropdown (no external dep). */
function DropdownMenu({
  button,
  children,
  align = 'left',
}: {
  button: (open: boolean) => React.ReactNode;
  children: (close: () => void) => React.ReactNode;
  align?: 'left' | 'right';
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <button type="button" onClick={() => setOpen((v) => !v)} className="flex items-center">
        {button(open)}
      </button>
      {open ? (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} aria-hidden />
          <div
            className={cn(
              'absolute top-full z-20 mt-2 min-w-56 rounded-2xl bg-popover p-2 shadow-xl shadow-foreground/10 animate-scale-in',
              align === 'right' ? 'right-0' : 'left-0',
            )}
          >
            {children(() => setOpen(false))}
          </div>
        </>
      ) : null}
    </div>
  );
}

function OrgSwitcher({
  currentSlug,
  currentName,
  logoUrl,
  role,
}: {
  currentSlug: string;
  currentName: string;
  logoUrl?: string;
  role: string;
}) {
  const { data: orgs } = useQuery({ queryKey: ['orgs'], queryFn: listMyOrgs });

  return (
    <DropdownMenu
      button={() => (
        <span className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors hover:bg-secondary">
          {logoUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={logoUrl} alt="" className="h-8 w-8 rounded-xl object-cover" />
          ) : (
            <span className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/12 text-primary">
              <Building2 className="h-3.5 w-3.5" />
            </span>
          )}
          <span className="hidden font-medium sm:block">{currentName}</span>
          <span className="hidden text-xs text-muted-foreground sm:block">{role.toLowerCase()}</span>
          <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
        </span>
      )}
    >
      {(close) => (
        <>
          <p className="px-2 py-1.5 text-xs font-medium text-muted-foreground">Organizations</p>
          {orgs?.map((o) => (
            <Link
              key={o.org_id}
              href={`/app/${o.slug}`}
              onClick={close}
              className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary"
            >
              <span className="truncate">{o.name}</span>
              {o.slug === currentSlug ? <Check className="h-4 w-4 shrink-0 text-primary" /> : null}
            </Link>
          ))}
          <div className="my-1 h-px bg-border" />
          <Link
            href="/app/new-organization"
            onClick={close}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary"
          >
            <Plus className="h-4 w-4" /> New organization
          </Link>
          <Link
            href="/app"
            onClick={close}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-secondary"
          >
            All organizations
          </Link>
        </>
      )}
    </DropdownMenu>
  );
}

function NotificationBell({ slug }: { slug: string }) {
  const { orgId } = useOrg();
  const { data: unread } = useQuery({
    queryKey: ['unread-count', orgId],
    queryFn: () => getUnreadCount(orgId),
  });

  return (
    <Link
      href={`/app/${slug}/notifications`}
      className="relative flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
      aria-label="Notifications"
    >
      <Bell className="h-4 w-4" />
      {unread && unread > 0 ? (
        <span className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-medium text-destructive-foreground animate-scale-in">
          {unread > 99 ? '99+' : unread}
        </span>
      ) : null}
    </Link>
  );
}

function AccountMenu() {
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe });
  const isPlatformAdmin = me?.platform_role === 'admin';

  return (
    <DropdownMenu
      align="right"
      button={() => (
        <span className="flex h-9 w-9 items-center justify-center">
          <Avatar name={me?.name} size="sm" />
        </span>
      )}
    >
      {(close) => (
        <>
          {me ? (
            <div className="px-2 py-2 text-sm">
              <p className="font-medium">{me.name}</p>
              <p className="text-xs text-muted-foreground">{me.email}</p>
            </div>
          ) : null}
          <div className="my-1 h-px bg-border" />
          <Link
            href="/account/profile"
            onClick={close}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary"
          >
            <User className="h-4 w-4" /> Account
          </Link>
          {isPlatformAdmin ? (
            <Link
              href="/admin"
              onClick={close}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-secondary"
            >
              <Shield className="h-4 w-4" /> Platform admin
            </Link>
          ) : null}
          <div className="my-1 h-px bg-border" />
          <Link
            href="/logout"
            onClick={close}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-destructive transition-colors hover:bg-destructive/10"
          >
            <LogOut className="h-4 w-4" /> Sign out
          </Link>
        </>
      )}
    </DropdownMenu>
  );
}
