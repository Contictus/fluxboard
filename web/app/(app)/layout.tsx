import type { ReactNode } from 'react';

import { RequireVerified } from '@/lib/auth/guards';

// User-scoped `(app)` group (account + org-selection). Every page here requires a
// verified session (`V` legend). The org-shell chrome under /app/{slug} is added
// in a later section; account pages render on a plain centered container.
export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <RequireVerified>
      <div className="min-h-screen">{children}</div>
    </RequireVerified>
  );
}
