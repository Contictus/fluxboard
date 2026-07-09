'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Building2, Bell, ChevronsUpDown, Plus, User, LogOut, Check } from 'lucide-react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';
import { listMyOrgs } from '@/lib/api/orgs';
import { getUnreadCount } from '@/lib/api/notifications';

export function Topbar() {
  const { org, slug, role } = useOrg();

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-2 border-b px-4">
      <OrgSwitcher currentSlug={slug} currentName={org.name} role={role} />
      <div className="flex items-center gap-1">
        <NotificationBell slug={slug} />
        <AccountMenu />
      </div>
    </header>
  );
}

/** Lightweight click-outside dropdown (no external dep). */
function Menu({
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
              'absolute top-full z-20 mt-1 min-w-56 rounded-md border bg-background p-1 shadow-md',
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
  role,
}: {
  currentSlug: string;
  currentName: string;
  role: string;
}) {
  const { data: orgs } = useQuery({ queryKey: ['orgs'], queryFn: listMyOrgs });

  return (
    <Menu
      button={() => (
        <span className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-secondary">
          <span className="flex h-6 w-6 items-center justify-center rounded bg-secondary">
            <Building2 className="h-3.5 w-3.5" />
          </span>
          <span className="font-medium">{currentName}</span>
          <span className="text-xs text-muted-foreground">{role.toLowerCase()}</span>
          <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
        </span>
      )}
    >
      {(close) => (
        <>
          <p className="px-2 py-1.5 text-xs text-muted-foreground">Organizations</p>
          {orgs?.map((o) => (
            <Link
              key={o.org_id}
              href={`/app/${o.slug}`}
              onClick={close}
              className="flex items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-secondary"
            >
              <span className="truncate">{o.name}</span>
              {o.slug === currentSlug ? <Check className="h-4 w-4 shrink-0" /> : null}
            </Link>
          ))}
          <div className="my-1 border-t" />
          <Link
            href="/app/new-organization"
            onClick={close}
            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-secondary"
          >
            <Plus className="h-4 w-4" /> New organization
          </Link>
          <Link
            href="/app"
            onClick={close}
            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-muted-foreground hover:bg-secondary"
          >
            All organizations
          </Link>
        </>
      )}
    </Menu>
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
      className="relative flex h-9 w-9 items-center justify-center rounded-md hover:bg-secondary"
      aria-label="Notifications"
    >
      <Bell className="h-4 w-4" />
      {unread && unread > 0 ? (
        <span className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-medium text-destructive-foreground">
          {unread > 99 ? '99+' : unread}
        </span>
      ) : null}
    </Link>
  );
}

function AccountMenu() {
  return (
    <Menu
      align="right"
      button={() => (
        <span className="flex h-9 w-9 items-center justify-center rounded-md hover:bg-secondary">
          <User className="h-4 w-4" />
        </span>
      )}
    >
      {(close) => (
        <>
          <Link
            href="/account/profile"
            onClick={close}
            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-secondary"
          >
            <User className="h-4 w-4" /> Account
          </Link>
          <Link
            href="/logout"
            onClick={close}
            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-secondary"
          >
            <LogOut className="h-4 w-4" /> Sign out
          </Link>
        </>
      )}
    </Menu>
  );
}
