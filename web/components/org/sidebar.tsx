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
  type LucideIcon,
} from 'lucide-react';

import { cn } from '@/lib/utils';
import { useOrg } from '@/lib/org/context';

interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
  /** Exact-match active (home), else prefix-match. */
  exact?: boolean;
}

// Role-aware org navigation. Links point at §5–§8 routes that land next sections;
// until then they resolve to Next's 404, which is acceptable placeholder behavior.
export function Sidebar() {
  const pathname = usePathname();
  const { slug, isAdmin } = useOrg();
  const base = `/app/${slug}`;

  const items: NavItem[] = [
    { href: base, label: 'Home', icon: Home, exact: true },
    { href: `${base}/projects`, label: 'Projects', icon: FolderKanban },
    { href: `${base}/my-tasks`, label: 'My tasks', icon: CheckSquare },
    { href: `${base}/search`, label: 'Search', icon: Search },
    { href: `${base}/trash`, label: 'Trash', icon: Trash2 },
  ];
  if (isAdmin) {
    items.push(
      { href: `${base}/settings/members`, label: 'Members', icon: Users },
      { href: `${base}/settings`, label: 'Settings', icon: Settings, exact: true },
      { href: `${base}/billing`, label: 'Billing', icon: CreditCard },
    );
  }

  return (
    <aside className="hidden w-56 shrink-0 border-r bg-secondary/30 md:block">
      <nav className="flex flex-col gap-1 p-3">
        {items.map((it) => {
          const active = it.exact ? pathname === it.href : pathname.startsWith(it.href);
          const Icon = it.icon;
          return (
            <Link
              key={it.href}
              href={it.href}
              className={cn(
                'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
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
    </aside>
  );
}
