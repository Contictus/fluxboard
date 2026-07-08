'use client';

// App-wide mount point for the 402 `plan_limit_exceeded` upgrade modal
// (docs/02 §9, FR-BILL-009). The trigger surface is wired here; the full modal
// body (plan comparison + checkout CTA) lands with Billing in Phase 8. For now
// it exposes the context so any mutation catching a 402 can raise it.

import { createContext, useContext, useState, type ReactNode } from 'react';

interface UpgradeModalValue {
  /** Raise the upgrade prompt; `limit` is the offending entitlement key. */
  open: (limit?: string) => void;
}

const UpgradeModalContext = createContext<UpgradeModalValue | null>(null);

export function UpgradeModalProvider({ children }: { children: ReactNode }) {
  const [limit, setLimit] = useState<string | null>(null);

  return (
    <UpgradeModalContext.Provider value={{ open: (l) => setLimit(l ?? 'plan_limit') }}>
      {children}
      {limit !== null ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => setLimit(null)}
        >
          <div
            className="w-full max-w-md rounded-lg border bg-card p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">Plan limit reached</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              You&apos;ve hit the <code className="font-mono">{limit}</code> limit on your current
              plan. Upgrade to continue. (Billing UI arrives in a later phase.)
            </p>
            <button
              className="mt-4 text-sm font-medium text-primary underline"
              onClick={() => setLimit(null)}
            >
              Dismiss
            </button>
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
