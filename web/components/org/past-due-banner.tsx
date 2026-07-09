'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AlertTriangle, X } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { getBillingSummary } from '@/lib/api/billing';

// Past-due warning banner (02 §9, 7.4.3). `billing/summary` is ADMIN+ gated, so
// this only fetches/renders for OWNER/ADMIN — MEMBER/GUEST would 403 (ADR-016).
export function PastDueBanner() {
  const { orgId, slug, isAdmin } = useOrg();
  const [dismissed, setDismissed] = useState(false);

  const { data } = useQuery({
    queryKey: ['billing-summary', orgId],
    queryFn: () => getBillingSummary(orgId),
    enabled: isAdmin,
  });

  if (!isAdmin || dismissed || !data?.past_due_warning) return null;

  return (
    <div className="flex items-center gap-3 border-b border-destructive/40 bg-destructive/10 px-4 py-2 text-sm">
      <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" />
      <p className="flex-1">
        Payment is past due. Update your billing details to keep your subscription active.
      </p>
      <Link
        href={`/app/${slug}/billing`}
        className="shrink-0 font-medium text-destructive underline-offset-2 hover:underline"
      >
        Fix billing
      </Link>
      <button
        type="button"
        onClick={() => setDismissed(true)}
        className="shrink-0 rounded p-1 hover:bg-destructive/20"
        aria-label="Dismiss"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}
