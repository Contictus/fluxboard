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
        'hidden shrink-0 flex-col bg-sidebar transition-all duration-300 md:flex',
        collapsed ? 'w-[72px]' : 'w-64',
      )}
    >
      {/* Logo area */}
      <div className={cn('flex h-20 items-center px-5', collapsed && 'justify-center')}>
        <Link href={base} className="flex items-center gap-2">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary font-display text-[11px] font-bold text-primary-foreground shadow-sm">
            f
          </span>
          {!collapsed ? <span className="font-display text-[15px] font-semibold tracking-tight text-sidebar-foreground">fluxboard</span> : null}
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-4 py-2">
        {/* Workspace section */}
        {!collapsed ? (
          <p className="mb-2 mt-4 px-3 text-[10px] font-semibold uppercase tracking-[0.16em] text-sidebar-foreground/40">
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
                <div className="my-5 h-px bg-sidebar-border/50" />
                <p className="mb-2 px-3 text-[10px] font-semibold uppercase tracking-[0.16em] text-sidebar-foreground/40">
                  Administration
                </p>
              </>
            ) : (
              <div className="my-5 h-px bg-sidebar-border/50" />
            )}
            {adminItems.map((it) => (
              <NavLink key={it.href} item={it} pathname={pathname} collapsed={collapsed} />
            ))}
          </>
        ) : null}
      </nav>

      {/* Bottom controls */}
      <div className={cn('flex items-center gap-1 p-4', collapsed ? 'flex-col' : 'justify-between')}>
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
        'group relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] transition-all duration-150',
        active
          ? 'bg-sidebar-foreground/10 font-medium text-sidebar-foreground shadow-sm'
          : 'text-sidebar-foreground/60 hover:bg-sidebar-foreground/5 hover:text-sidebar-foreground',
        collapsed && 'justify-center px-0',
      )}
    >
      {/* Active indicator bar */}
      {active ? (
        <span className="absolute left-1 top-1/2 h-5 w-1 -translate-y-1/2 rounded-full bg-sidebar-active" />
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
      <aside className="fixed inset-y-0 left-0 z-40 w-72 bg-sidebar shadow-2xl animate-slide-in-left md:hidden">
        <div className="flex h-20 items-center gap-2 px-5">
          <span className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary text-xs font-black text-primary-foreground">
            f
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
                  'flex items-center gap-2 rounded-xl px-3 py-2.5 text-sm transition-colors',
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
