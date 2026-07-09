'use client';

import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, ExternalLink } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useToast } from '@/components/ui/toast';
import { useOrg } from '@/lib/org/context';
import { useOrgMembers } from '@/lib/org/use-members';
import {
  billingPortal,
  cancelSubscription,
  getBillingSummary,
  resumeSubscription,
} from '@/lib/api/billing';
import { planByCode, limitLabel } from '@/lib/billing/plans';
import type { SubStatus } from '@/lib/api/types';

const STATUS_STYLE: Record<SubStatus, string> = {
  active: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  trialing: 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
  past_due: 'bg-amber-500/15 text-amber-600 dark:text-amber-500',
  unpaid: 'bg-destructive/15 text-destructive',
  canceled: 'bg-secondary text-muted-foreground',
  none: 'bg-secondary text-muted-foreground',
};

function fmtDate(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

export default function BillingOverviewPage() {
  const { orgId, slug } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { members } = useOrgMembers();

  const summary = useQuery({
    queryKey: ['billing-summary', orgId],
    queryFn: () => getBillingSummary(orgId),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['billing-summary', orgId] });

  const cancel = useMutation({
    mutationFn: () => cancelSubscription(orgId, true),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Cancellation scheduled for period end', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t cancel', variant: 'error' }),
  });

  const resume = useMutation({
    mutationFn: () => resumeSubscription(orgId),
    onSuccess: () => {
      invalidate();
      toast({ title: 'Subscription resumed', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t resume', variant: 'error' }),
  });

  const portal = useMutation({
    mutationFn: () => billingPortal(orgId),
    onSuccess: (url) => {
      window.location.href = url;
    },
    onError: () => toast({ title: 'Couldn’t open the billing portal', variant: 'error' }),
  });

  if (summary.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading billing…</p>;
  }
  if (summary.isError || !summary.data) {
    return <p className="text-sm text-destructive">Couldn’t load billing information.</p>;
  }

  const s = summary.data;
  const plan = planByCode(s.plan);
  const status = s.status;
  const seatCount = members.length;
  const maxMembers = s.entitlements.max_members;

  return (
    <div className="space-y-6">
      {s.past_due_warning ? (
        <div className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-4 text-sm">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
          <div>
            <p className="font-medium">Payment past due</p>
            <p className="text-muted-foreground">
              Your last payment failed. Update your payment method to avoid losing access.
            </p>
            <Button
              size="sm"
              className="mt-2"
              onClick={() => portal.mutate()}
              disabled={portal.isPending}
            >
              Update payment method
            </Button>
          </div>
        </div>
      ) : null}

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <CardTitle>{plan?.name ?? s.plan} plan</CardTitle>
          <span
            className={`rounded-full px-2.5 py-1 text-xs font-medium capitalize ${STATUS_STYLE[status]}`}
          >
            {status.replace('_', ' ')}
          </span>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">Seats</dt>
              <dd className="mt-0.5 font-medium">
                {seatCount} / {limitLabel(maxMembers)}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">
                {s.cancel_at_period_end ? 'Ends' : 'Renews'}
              </dt>
              <dd className="mt-0.5 font-medium">{fmtDate(s.current_period_end)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Metered usage</dt>
              <dd className="mt-0.5 font-medium">
                {s.entitlements.metered ? 'On' : 'Off'}
              </dd>
            </div>
          </dl>

          {s.cancel_at_period_end ? (
            <div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-sm">
              <p className="text-muted-foreground">
                This subscription is set to cancel on {fmtDate(s.current_period_end)}. You can
                resume it before then.
              </p>
              <Button
                size="sm"
                className="mt-2"
                onClick={() => resume.mutate()}
                disabled={resume.isPending}
              >
                {resume.isPending ? 'Resuming…' : 'Resume subscription'}
              </Button>
            </div>
          ) : null}

          <div className="flex flex-wrap gap-2 pt-1">
            <Link
              href={`/app/${slug}/billing/plans`}
              className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
            >
              {s.has_subscription ? 'Change plan' : 'Choose a plan'}
            </Link>
            {s.has_subscription ? (
              <>
                <Button
                  variant="outline"
                  onClick={() => portal.mutate()}
                  disabled={portal.isPending}
                >
                  Manage payment method <ExternalLink className="h-3.5 w-3.5" />
                </Button>
                {!s.cancel_at_period_end && status !== 'canceled' ? (
                  <Button
                    variant="ghost"
                    className="text-destructive hover:text-destructive"
                    onClick={() => cancel.mutate()}
                    disabled={cancel.isPending}
                  >
                    {cancel.isPending ? 'Cancelling…' : 'Cancel subscription'}
                  </Button>
                ) : null}
              </>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <p className="text-xs text-muted-foreground">
        Card details are handled entirely by Stripe — they never touch our servers. Manage your
        payment method through the secure billing portal.
      </p>
    </div>
  );
}
