'use client';

import Link from 'next/link';
import { XCircle } from 'lucide-react';

import { useOrg } from '@/lib/org/context';

// Checkout cancelled landing (docs/06 §3). Stripe redirects here when the user
// abandons the hosted checkout. Nothing changed — offer a way back to plans.
export default function CheckoutCancelledPage() {
  const { slug } = useOrg();

  return (
    <div className="mx-auto flex max-w-md flex-col items-center py-16 text-center">
      <XCircle className="h-12 w-12 text-muted-foreground" />
      <h1 className="mt-4 text-xl font-semibold">Checkout cancelled</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        No charge was made and your plan is unchanged. You can pick a plan whenever you’re ready.
      </p>
      <div className="mt-6 flex gap-2">
        <Link
          href={`/app/${slug}/billing/plans`}
          className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          Back to plans
        </Link>
        <Link
          href={`/app/${slug}/billing`}
          className="inline-flex h-10 items-center justify-center rounded-md border border-input bg-background px-4 text-sm font-medium hover:bg-secondary"
        >
          Billing overview
        </Link>
      </div>
    </div>
  );
}
