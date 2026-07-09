'use client';

// App-wide mount point for the 402 `plan_limit_exceeded` upgrade modal
// (docs/02 §9, FR-BILL-009). Any mutation that catches a 402 calls
// `useUpgradeModal().open(limitKey)` to raise it. The modal lives above the org
// context (it wraps the whole app in providers.tsx), so it derives the org slug
// from the pathname to link into that org's plans page.

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { createContext, useContext, useState, type ReactNode } from 'react';

interface UpgradeModalValue {
  /** Raise the upgrade prompt; `limit` is the offending entitlement key. */
  open: (limit?: string) => void;
}

const UpgradeModalContext = createContext<UpgradeModalValue | null>(null);

// Friendly copy for the entitlement keys the API reports on a 402.
const LIMIT_COPY: Record<string, string> = {
  max_members: 'You’ve reached the member limit for your current plan.',
  max_projects: 'You’ve reached the project limit for your current plan.',
  max_storage_bytes: 'You’ve reached the storage limit for your current plan.',
  members: 'You’ve reached the member limit for your current plan.',
  seats: 'You’ve reached the seat limit for your current plan.',
  plan_limit: 'You’ve reached a limit on your current plan.',
};

function slugFromPath(pathname: string): string | null {
  const m = pathname.match(/^\/app\/([^/]+)/);
  const slug = m?.[1];
  if (!slug || slug === 'new-organization') return null;
  return slug;
}

export function UpgradeModalProvider({ children }: { children: ReactNode }) {
  const [limit, setLimit] = useState<string | null>(null);
  const pathname = usePathname();
  const slug = slugFromPath(pathname);

  const close = () => setLimit(null);

  return (
    <UpgradeModalContext.Provider value={{ open: (l) => setLimit(l ?? 'plan_limit') }}>
      {children}
      {limit !== null ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={close}
        >
          <div
            className="w-full max-w-md rounded-lg border bg-card p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">Upgrade your plan</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              {LIMIT_COPY[limit] ?? LIMIT_COPY.plan_limit} Upgrade to raise your limits and keep
              going.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <button
                className="inline-flex h-10 items-center justify-center rounded-md px-4 text-sm font-medium hover:bg-secondary"
                onClick={close}
              >
                Not now
              </button>
              {slug ? (
                <Link
                  href={`/app/${slug}/billing/plans`}
                  onClick={close}
                  className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                >
                  View plans
                </Link>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </UpgradeModalContext.Provider>
  );
}

export function useUpgradeModal(): UpgradeModalValue {
  const ctx = useContext(UpgradeModalContext);
  if (!ctx) throw new Error('useUpgradeModal must be used within <UpgradeModalProvider>');
  return ctx;
}
