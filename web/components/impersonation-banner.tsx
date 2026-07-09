'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { ShieldAlert } from 'lucide-react';

import { useAuth } from '@/lib/auth/context';

// App-wide read-only impersonation banner (FR-ADM-003). When the access token
// carries an `imp` claim, an admin is viewing a tenant as one of its members with
// READ-only access (the server rejects any write). Exiting refreshes the session
// cookie back into the admin's own token — the `imp` token was only ever held in
// memory, never a cookie, so a plain refresh restores the real identity.
export function ImpersonationBanner() {
  const { impersonating, refresh } = useAuth();
  const router = useRouter();
  const qc = useQueryClient();
  const [exiting, setExiting] = useState(false);

  if (!impersonating) return null;

  const exit = async () => {
    setExiting(true);
    await refresh();
    qc.clear();
    router.push('/admin');
  };

  return (
    <div className="flex items-center justify-center gap-3 bg-amber-500 px-4 py-2 text-sm font-medium text-neutral-950">
      <ShieldAlert className="h-4 w-4" />
      <span>Read-only impersonation session — changes are disabled.</span>
      <button
        onClick={() => void exit()}
        disabled={exiting}
        className="rounded-md bg-neutral-950/20 px-3 py-0.5 text-xs font-semibold hover:bg-neutral-950/30 disabled:opacity-60"
      >
        {exiting ? 'Exiting…' : 'Exit'}
      </button>
    </div>
  );
}
