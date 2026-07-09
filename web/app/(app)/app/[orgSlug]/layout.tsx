import type { ReactNode } from 'react';

import { OrgProvider } from '@/lib/org/context';
import { OrgShell } from '@/components/org/shell';

// Org shell (Phase 7 §4). Mounts under the (app) group's RequireVerified guard.
// The static `new-organization/` sibling wins over this dynamic segment, so
// `/app/new-organization` never resolves here.
export default function OrgLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: { orgSlug: string };
}) {
  return (
    <OrgProvider slug={params.orgSlug}>
      <OrgShell>{children}</OrgShell>
    </OrgProvider>
  );
}
