'use client';

import Link from 'next/link';
import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, Loader2 } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { getBillingSummary } from '@/lib/api/billing';

// Checkout success landing (docs/06 §3). Stripe redirects here after payment, but
// the subscription is only real once the webhook lands, so we poll the summary
// until it reports an active subscription (or a 60s timeout, then offer a manual
// check). The webhook is the source of truth — this page never trusts the redirect.
const POLL_MS = 2000;
const TIMEOUT_MS = 60_000;

export default function CheckoutSuccessPage() {
  const { orgId, slug } = useOrg();
  const startedAt = useRef(Date.now());
  const [timedOut, setTimedOut] = useState(false);

  const summary = useQuery({
    queryKey: ['billing-summary', orgId],
    queryFn: () => getBillingSummary(orgId),
    refetchInterval: (query) => {
      const done = query.state.data?.has_subscription;
      if (done || timedOut) return false;
      return POLL_MS;
    },
  });

  useEffect(() => {
    const t = setTimeout(() => setTimedOut(true), TIMEOUT_MS);
    return () => clearTimeout(t);
  }, []);

  useEffect(() => {
    if (Date.now() - startedAt.current > TIMEOUT_MS) setTimedOut(true);
  }, [summary.dataUpdatedAt]);

  const active = summary.data?.has_subscription;

  return (
    <div className="mx-auto flex max-w-md flex-col items-center py-16 text-center">
      {active ? (
        <>
          <CheckCircle2 className="h-12 w-12 text-emerald-500" />
          <h1 className="mt-4 text-xl font-semibold">You’re all set</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Your subscription is active. Welcome aboard.
          </p>
          <Link
            href={`/app/${slug}/billing`}
            className="mt-6 inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
          >
            Go to billing
          </Link>
        </>
      ) : timedOut ? (
        <>
          <h1 className="text-xl font-semibold">Almost there</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Your payment went through, but we’re still finalizing your subscription. This can take a
            moment — refresh to check again.
          </p>
          <button
            onClick={() => {
              setTimedOut(false);
              startedAt.current = Date.now();
              void summary.refetch();
            }}
            className="mt-6 inline-flex h-10 items-center justify-center rounded-md border border-input bg-background px-4 text-sm font-medium hover:bg-secondary"
          >
            Check again
          </button>
        </>
      ) : (
        <>
          <Loader2 className="h-10 w-10 animate-spin text-muted-foreground" />
          <h1 className="mt-4 text-xl font-semibold">Finalizing your subscription…</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Payment received. We’re activating your plan — this usually takes a few seconds.
          </p>
        </>
      )}
    </div>
  );
}
