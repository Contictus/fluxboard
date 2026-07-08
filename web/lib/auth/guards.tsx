'use client';

import { useRouter } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';

import { useAuth } from './context';

function Loading() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center text-sm text-muted-foreground">
      Loading…
    </div>
  );
}

/**
 * Gate for the `A` legend: requires a live session. Redirects to /login (with a
 * post-login `next`) once the bootstrap resolves without a token.
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { loading, isAuthenticated } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (loading) return;
    if (!isAuthenticated) {
      const next = encodeURIComponent(window.location.pathname + window.location.search);
      router.replace(`/login?next=${next}`);
    }
  }, [loading, isAuthenticated, router]);

  if (loading) return <Loading />;
  if (!isAuthenticated) return <Loading />;
  return <>{children}</>;
}

/**
 * Gate for the `V` legend: requires a session AND a verified email. Unverified
 * users are sent to the blocking /verify-email screen.
 */
export function RequireVerified({ children }: { children: ReactNode }) {
  const { loading, isAuthenticated, verified } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (loading) return;
    if (!isAuthenticated) {
      const next = encodeURIComponent(window.location.pathname + window.location.search);
      router.replace(`/login?next=${next}`);
    } else if (!verified) {
      router.replace('/verify-email');
    }
  }, [loading, isAuthenticated, verified, router]);

  if (loading || !isAuthenticated || !verified) return <Loading />;
  return <>{children}</>;
}

/**
 * Gate for the `PA` legend. platform_role is NOT in the access token — it comes
 * from GET /me (added in §B). Until that wiring lands this guard only ensures a
 * session; the admin surface itself is out of scope for this run.
 */
export function RequirePlatformAdmin({ children }: { children: ReactNode }) {
  return <RequireAuth>{children}</RequireAuth>;
}
