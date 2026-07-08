import type { Metadata } from 'next';

import { PricingTable } from '@/components/marketing/pricing-table';

export const metadata: Metadata = { title: 'Pricing' };

const faqs = [
  {
    q: 'How does metered billing work?',
    a: 'Your base plan includes seats, storage, and API-call allowances. Usage beyond the included amount is metered and reported to Stripe, so your invoice reflects what you actually consumed each cycle.',
  },
  {
    q: 'What counts as a seat?',
    a: 'Any active member of an organization. Guests with read-only access to a single project do not consume a seat.',
  },
  {
    q: 'Can I change plans anytime?',
    a: 'Yes. Upgrades apply immediately with prorated charges; downgrades take effect at the end of the current billing period.',
  },
  {
    q: 'What happens if I hit a plan limit?',
    a: 'Writes that would exceed a hard limit return a clear upgrade prompt rather than failing silently. Reads are never blocked.',
  },
];

export default function PricingPage() {
  return (
    <div className="container py-16">
      <div className="mx-auto mb-12 max-w-2xl text-center">
        <h1 className="text-4xl font-bold tracking-tight">Pricing</h1>
        <p className="mt-3 text-muted-foreground">
          Transparent plans with usage-based metering. No surprises — you only pay for the seats,
          storage, and API calls beyond your plan&apos;s included allowance.
        </p>
      </div>

      <PricingTable />

      <div className="mx-auto mt-16 max-w-3xl rounded-lg border bg-muted/30 p-6">
        <h2 className="text-lg font-semibold">How metered billing works</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Every plan bundles an allowance of seats, storage, and API calls. We meter usage
          continuously and report it to Stripe. At the end of each cycle your invoice combines the
          flat plan fee with any metered overage, itemized so you can see exactly where costs came
          from. A usage dashboard shows live meters versus limits and an estimated invoice.
        </p>
      </div>

      <div className="mx-auto mt-16 max-w-3xl">
        <h2 className="text-2xl font-bold tracking-tight">Frequently asked questions</h2>
        <dl className="mt-6 space-y-6">
          {faqs.map((f) => (
            <div key={f.q}>
              <dt className="font-medium">{f.q}</dt>
              <dd className="mt-1 text-sm text-muted-foreground">{f.a}</dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}
