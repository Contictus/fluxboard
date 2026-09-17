'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  Home,
  FolderKanban,
  CheckSquare,
  Search,
  Trash2,
  Users,
  Settings,
  CreditCard,
  PanelLeftClose,
  PanelLeft,
  type LucideIcon,
} from 'lucide-react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';
import { ThemeToggle } from '@/components/ui/theme-toggle';
import { Tooltip } from '@/components/ui/tooltip';

interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
  /** Exact-match active (home), else prefix-match. */
  exact?: boolean;
  section?: 'workspace' | 'admin';
}

// Role-aware org navigation. Links point at §5–§8 routes that land next sections;
// until then they resolve to Next's 404, which is acceptable placeholder behavior.
export function Sidebar({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  const pathname = usePathname();
  const { slug, isAdmin } = useOrg();
  const base = `/app/${slug}`;

  const items: NavItem[] = [
    { href: base, label: 'Home', icon: Home, exact: true, section: 'workspace' },
    { href: `${base}/projects`, label: 'Projects', icon: FolderKanban, section: 'workspace' },
    { href: `${base}/my-tasks`, label: 'My tasks', icon: CheckSquare, section: 'workspace' },
    { href: `${base}/search`, label: 'Search', icon: Search, section: 'workspace' },
    { href: `${base}/trash`, label: 'Trash', icon: Trash2, section: 'workspace' },
  ];
  if (isAdmin) {
    items.push(
      { href: `${base}/settings/members`, label: 'Members', icon: Users, section: 'admin' },
      { href: `${base}/settings`, label: 'Settings', icon: Settings, exact: true, section: 'admin' },
      { href: `${base}/billing`, label: 'Billing', icon: CreditCard, section: 'admin' },
    );
  }

  const workspaceItems = items.filter((i) => i.section === 'workspace');
  const adminItems = items.filter((i) => i.section === 'admin');

  return (
    <aside
      className={cn(
        'hidden shrink-0 flex-col border-r bg-sidebar transition-all duration-300 md:flex',
        collapsed ? 'w-[60px]' : 'w-56',
      )}
    >
      {/* Logo area */}
      <div className={cn('flex h-14 items-center border-b border-sidebar-border px-3', collapsed && 'justify-center')}>
        <Link href={base} className="flex items-center gap-2">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary text-xs font-black text-primary-foreground">
            F
          </span>
          {!collapsed ? <span className="text-sm font-bold tracking-tight text-sidebar-foreground">Fluxboard</span> : null}
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex flex-1 flex-col gap-1 overflow-y-auto p-2">
        {/* Workspace section */}
        {!collapsed ? (
          <p className="mb-1 mt-2 px-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Workspace
          </p>
        ) : null}
        {workspaceItems.map((it) => (
          <NavLink key={it.href} item={it} pathname={pathname} collapsed={collapsed} />
        ))}

        {/* Admin section */}
        {adminItems.length > 0 ? (
          <>
            {!collapsed ? (
              <>
                <div className="my-2 h-px bg-sidebar-border" />
                <p className="mb-1 px-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Administration
                </p>
              </>
            ) : (
              <div className="my-2 h-px bg-sidebar-border" />
            )}
            {adminItems.map((it) => (
              <NavLink key={it.href} item={it} pathname={pathname} collapsed={collapsed} />
            ))}
          </>
        ) : null}
      </nav>

      {/* Bottom controls */}
      <div className={cn('flex items-center gap-1 border-t border-sidebar-border p-2', collapsed ? 'flex-col' : 'justify-between')}>
        <ThemeToggle compact />
        <button
          type="button"
          onClick={onToggle}
          className="flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <PanelLeft className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
        </button>
      </div>
    </aside>
  );
}

function NavLink({
  item,
  pathname,
  collapsed,
}: {
  item: NavItem;
  pathname: string;
  collapsed: boolean;
}) {
  const active = item.exact ? pathname === item.href : pathname.startsWith(item.href);
  const Icon = item.icon;

  const link = (
    <Link
      href={item.href}
      className={cn(
        'group relative flex items-center gap-2 rounded-md px-2.5 py-2 text-sm transition-all duration-200',
        active
          ? 'bg-secondary font-medium text-foreground'
          : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
        collapsed && 'justify-center px-0',
      )}
    >
      {/* Active indicator bar */}
      {active ? (
        <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-sidebar-active" />
      ) : null}
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed ? <span className="truncate">{item.label}</span> : null}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip content={item.label} side="bottom">
        {link}
      </Tooltip>
    );
  }

  return link;
}

/** Mobile sidebar overlay */
export function MobileSidebar({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const pathname = usePathname();
  const { slug, isAdmin } = useOrg();
  const base = `/app/${slug}`;

  const items: NavItem[] = [
    { href: base, label: 'Home', icon: Home, exact: true, section: 'workspace' },
    { href: `${base}/projects`, label: 'Projects', icon: FolderKanban, section: 'workspace' },
    { href: `${base}/my-tasks`, label: 'My tasks', icon: CheckSquare, section: 'workspace' },
    { href: `${base}/search`, label: 'Search', icon: Search, section: 'workspace' },
    { href: `${base}/trash`, label: 'Trash', icon: Trash2, section: 'workspace' },
  ];
  if (isAdmin) {
    items.push(
      { href: `${base}/settings/members`, label: 'Members', icon: Users, section: 'admin' },
      { href: `${base}/settings`, label: 'Settings', icon: Settings, exact: true, section: 'admin' },
      { href: `${base}/billing`, label: 'Billing', icon: CreditCard, section: 'admin' },
    );
  }

  if (!open) return null;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm animate-fade-in md:hidden"
        onClick={onClose}
        aria-hidden
      />
      {/* Drawer */}
      <aside className="fixed inset-y-0 left-0 z-40 w-64 border-r bg-sidebar shadow-2xl animate-slide-in-left md:hidden">
        <div className="flex h-14 items-center gap-2 border-b border-sidebar-border px-4">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-xs font-black text-primary-foreground">
            F
          </span>
          <span className="font-bold tracking-tight text-sidebar-foreground">Fluxboard</span>
        </div>
        <nav className="flex flex-col gap-1 p-3">
          {items.map((it) => {
            const active = it.exact ? pathname === it.href : pathname.startsWith(it.href);
            const Icon = it.icon;
            return (
              <Link
                key={it.href}
                href={it.href}
                onClick={onClose}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2.5 text-sm transition-colors',
                  active
                    ? 'bg-secondary font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
                )}
              >
                <Icon className="h-4 w-4" />
                {it.label}
              </Link>
            );
          })}
        </nav>
        <div className="absolute bottom-0 left-0 right-0 border-t border-sidebar-border p-3">
          <ThemeToggle />
        </div>
      </aside>
    </>
  );
}
