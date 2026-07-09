import type { ReactNode } from 'react';

import { RequireVerified } from '@/lib/auth/guards';
import { ImpersonationBanner } from '@/components/impersonation-banner';

// User-scoped `(app)` group (account + org-selection). Every page here requires a
// verified session (`V` legend). A read-only impersonation banner sits above all
// app chrome whenever an admin holds an `imp`-claim token (FR-ADM-003).
export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <RequireVerified>
      <div className="min-h-screen">
        <ImpersonationBanner />
        {children}
      </div>
    </RequireVerified>
  );
}
