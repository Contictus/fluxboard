import Link from 'next/link';
import { Kanban, Users, CreditCard, Activity, ShieldCheck, Zap } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { PricingTable } from '@/components/marketing/pricing-table';

const features = [
  { icon: Kanban, title: 'Kanban boards', body: 'Drag-drop tasks with WIP limits and live board sync.' },
  { icon: Users, title: 'Multi-tenant teams', body: 'Organizations, roles, and invitations with strict isolation.' },
  { icon: CreditCard, title: 'Usage-based billing', body: 'Metered seats, storage, and API calls via Stripe.' },
  { icon: Activity, title: 'Realtime activity', body: 'Server-sent events push updates the moment they happen.' },
  { icon: ShieldCheck, title: 'Secure by default', body: 'Row-level tenant isolation, 2FA, and audit logging.' },
  { icon: Zap, title: 'Fast API', body: 'A typed REST API with an OpenAPI spec you can build on.' },
];

export default function LandingPage() {
  return (
    <>
      <section className="container flex flex-col items-center gap-6 py-24 text-center">
        <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">
          Project management + metered billing
        </span>
        <h1 className="max-w-3xl text-4xl font-bold tracking-tight sm:text-5xl">
          Ship work faster with boards, teams, and billing that just work.
        </h1>
        <p className="max-w-xl text-lg text-muted-foreground">
          Fluxboard combines a Linear-style task tracker with Stripe-powered usage billing,
          built for teams that need multi-tenant isolation out of the box.
        </p>
        <div className="flex gap-3">
          <Link href="/register">
            <Button size="lg">Start for free</Button>
          </Link>
          <Link href="/features">
            <Button size="lg" variant="outline">
              See features
            </Button>
          </Link>
        </div>
      </section>

      <section className="container py-16">
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {features.map((f) => (
            <div key={f.title} className="rounded-lg border bg-card p-6">
              <f.icon className="h-6 w-6 text-primary" />
              <h3 className="mt-4 font-semibold">{f.title}</h3>
              <p className="mt-1 text-sm text-muted-foreground">{f.body}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="container py-16">
        <div className="mx-auto mb-10 max-w-xl text-center">
          <h2 className="text-3xl font-bold tracking-tight">Simple, metered pricing</h2>
          <p className="mt-2 text-muted-foreground">
            Start free. Pay for the seats, storage, and API you actually use.
          </p>
        </div>
        <PricingTable />
      </section>

      <section className="container py-24 text-center">
        <h2 className="text-3xl font-bold tracking-tight">Ready to get organized?</h2>
        <p className="mt-2 text-muted-foreground">Create your workspace in under a minute.</p>
        <Link href="/register" className="mt-6 inline-block">
          <Button size="lg">Create your workspace</Button>
        </Link>
      </section>
    </>
  );
}
