'use client';

import { useCallback, useEffect, useState, type ReactNode } from 'react';

import { useOrg } from '@/lib/org/context';
import { Sidebar, MobileSidebar } from './sidebar';
import { Topbar } from './topbar';
import { PastDueBanner } from './past-due-banner';
import { SoftDeleteScreen } from './soft-delete-screen';
import { Realtime } from './realtime';

const SIDEBAR_KEY = 'fluxboard-sidebar-collapsed';

// Chrome for the org shell. A soft-deleted org takes over the whole viewport with
// the restore screen; otherwise the standard sidebar + topbar frame renders.
export function OrgShell({ children }: { children: ReactNode }) {
  const { org } = useOrg();
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  // Restore sidebar state from localStorage
  useEffect(() => {
    const stored = localStorage.getItem(SIDEBAR_KEY);
    if (stored === 'true') setCollapsed(true);
  }, []);

  const toggleSidebar = useCallback(() => {
    setCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(SIDEBAR_KEY, String(next));
      return next;
    });
  }, []);

  if (org.deleted_at) return <SoftDeleteScreen />;

  return (
    <div className="flex min-h-screen">
      <Realtime />
      <Sidebar collapsed={collapsed} onToggle={toggleSidebar} />
      <MobileSidebar open={mobileOpen} onClose={() => setMobileOpen(false)} />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar onMobileMenuToggle={() => setMobileOpen((v) => !v)} />
        <PastDueBanner />
        <main className="min-w-0 flex-1 animate-fade-in">{children}</main>
      </div>
    </div>
  );
}
