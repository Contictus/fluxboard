import Link from 'next/link';
import { Check } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { plans, formatPrice } from '@/lib/marketing/plans';

export function PricingTable() {
  return (
    <div className="grid gap-6 md:grid-cols-3">
      {plans.map((plan) => (
        <div
          key={plan.key}
          className={cn(
            'flex flex-col rounded-lg border bg-card p-6',
            plan.highlighted && 'border-primary shadow-md ring-1 ring-primary',
          )}
        >
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">{plan.name}</h3>
            {plan.highlighted ? (
              <span className="rounded-full bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground">
                Popular
              </span>
            ) : null}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{plan.tagline}</p>
          <p className="mt-4">
            <span className="text-3xl font-bold">{formatPrice(plan.priceCents)}</span>
            <span className="text-sm text-muted-foreground">/mo</span>
          </p>
          <ul className="mt-6 flex-1 space-y-2">
            {plan.features.map((f) => (
              <li key={f} className="flex items-start gap-2 text-sm">
                <Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                <span>{f}</span>
              </li>
            ))}
          </ul>
          <Link href="/register" className="mt-6">
            <Button className="w-full" variant={plan.highlighted ? 'default' : 'outline'}>
              Get started
            </Button>
          </Link>
        </div>
      ))}
    </div>
  );
}
