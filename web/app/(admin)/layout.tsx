import type { ReactNode } from 'react';

import { RequirePlatformAdmin } from '@/lib/auth/guards';

// Platform-admin `(admin)` group — separate dark chrome (docs/02 §7). Body pages
// are out of scope for this run; the guarded shell exists from scaffolding.
export default function AdminLayout({ children }: { children: ReactNode }) {
  return (
    <RequirePlatformAdmin>
      <div className="min-h-screen bg-slate-950 text-slate-100">{children}</div>
    </RequirePlatformAdmin>
  );
}
