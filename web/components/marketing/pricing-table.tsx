import Link from 'next/link';
import { Check, Sparkles } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { plans, formatPrice } from '@/lib/marketing/plans';

export function PricingTable() {
  return (
    <div className="stagger-children grid gap-6 md:grid-cols-3">
      {plans.map((plan) => (
        <div
          key={plan.key}
          className={cn(
            'group relative flex flex-col rounded-xl border bg-card p-6 transition-all duration-300 hover:-translate-y-1 hover:shadow-xl',
            plan.highlighted && 'border-primary/50 shadow-lg glow-primary',
          )}
        >
          {plan.highlighted ? (
            <span className="absolute -top-3 left-1/2 -translate-x-1/2 flex items-center gap-1 rounded-full bg-primary px-3 py-1 text-xs font-medium text-primary-foreground shadow-md">
              <Sparkles className="h-3 w-3" />
              Most popular
            </span>
          ) : null}
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">{plan.name}</h3>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{plan.tagline}</p>
          <p className="mt-5">
            <span className="text-4xl font-bold tracking-tight">{formatPrice(plan.priceCents)}</span>
            <span className="text-sm text-muted-foreground">/mo</span>
          </p>
          <ul className="mt-6 flex-1 space-y-2.5">
            {plan.features.map((f) => (
              <li key={f} className="flex items-start gap-2.5 text-sm">
                <Check className="mt-0.5 h-4 w-4 shrink-0 text-success" />
                <span>{f}</span>
              </li>
            ))}
          </ul>
          <Link href="/register" className="mt-8">
            <Button
              className={cn(
                'w-full',
                plan.highlighted && 'shadow-md',
              )}
              variant={plan.highlighted ? 'default' : 'outline'}
            >
              Get started
            </Button>
          </Link>
        </div>
      ))}
    </div>
  );
}
