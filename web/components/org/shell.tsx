'use client';

import type { ReactNode } from 'react';

import { useOrg } from '@/lib/org/context';
import { Sidebar } from './sidebar';
import { Topbar } from './topbar';
import { PastDueBanner } from './past-due-banner';
import { SoftDeleteScreen } from './soft-delete-screen';
import { Realtime } from './realtime';

// Chrome for the org shell. A soft-deleted org takes over the whole viewport with
// the restore screen; otherwise the standard sidebar + topbar frame renders.
export function OrgShell({ children }: { children: ReactNode }) {
  const { org } = useOrg();

  if (org.deleted_at) return <SoftDeleteScreen />;

  return (
    <div className="flex min-h-screen">
      <Realtime />
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <PastDueBanner />
        <main className="min-w-0 flex-1">{children}</main>
      </div>
    </div>
  );
}
