import Link from 'next/link';
import { Kanban, Users, CreditCard, Activity, ShieldCheck, Zap, ArrowRight } from 'lucide-react';

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

const stats = [
  { value: '99.9%', label: 'Uptime SLA' },
  { value: '<50ms', label: 'API latency' },
  { value: '∞', label: 'Projects' },
];

export default function LandingPage() {
  return (
    <>
      {/* ── Hero ─────────────────────────────────────── */}
      <section className="relative overflow-hidden">
        {/* Background mesh */}
        <div className="absolute inset-0 -z-10 bg-gradient-to-b from-primary/5 via-background to-background" />
        <div className="absolute -top-40 left-1/2 -z-10 h-[500px] w-[800px] -translate-x-1/2 rounded-full bg-primary/10 blur-3xl" />
        <div className="absolute -top-20 left-1/4 -z-10 h-[300px] w-[400px] rounded-full bg-accent/10 blur-3xl" />

        <div className="container flex flex-col items-center gap-6 py-24 text-center md:py-32">
          <span className="animate-fade-in rounded-full border border-primary/20 bg-primary/5 px-4 py-1.5 text-xs font-medium text-primary">
            Project management + metered billing
          </span>
          <h1 className="max-w-3xl animate-slide-up text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl">
            Ship work faster with boards, teams, and billing that{' '}
            <span className="gradient-text">just work</span>.
          </h1>
          <p className="max-w-xl animate-slide-up text-lg text-muted-foreground" style={{ animationDelay: '100ms' }}>
            Fluxboard combines a Linear-style task tracker with Stripe-powered usage billing,
            built for teams that need multi-tenant isolation out of the box.
          </p>
          <div className="flex gap-3 animate-slide-up" style={{ animationDelay: '200ms' }}>
            <Link href="/register">
              <Button size="lg" className="gap-2">
                Start for free
                <ArrowRight className="h-4 w-4" />
              </Button>
            </Link>
            <Link href="/features">
              <Button size="lg" variant="outline">
                See features
              </Button>
            </Link>
          </div>
        </div>
      </section>

      {/* ── Stats band ───────────────────────────────── */}
      <section className="border-y bg-secondary/30">
        <div className="container flex items-center justify-center gap-12 py-8 md:gap-20">
          {stats.map((s) => (
            <div key={s.label} className="text-center">
              <p className="text-2xl font-bold tracking-tight md:text-3xl">{s.value}</p>
              <p className="mt-1 text-xs text-muted-foreground">{s.label}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ── Features ─────────────────────────────────── */}
      <section className="container py-20">
        <div className="mx-auto mb-12 max-w-xl text-center">
          <h2 className="text-3xl font-bold tracking-tight">
            Everything your team needs
          </h2>
          <p className="mt-3 text-muted-foreground">
            From task tracking to usage-based billing — all in one platform.
          </p>
        </div>
        <div className="stagger-children grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {features.map((f) => (
            <div
              key={f.title}
              className="group rounded-xl border bg-card p-6 transition-all duration-300 hover:shadow-lg hover:-translate-y-0.5"
            >
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary transition-colors group-hover:bg-primary group-hover:text-primary-foreground">
                <f.icon className="h-5 w-5" />
              </div>
              <h3 className="mt-4 font-semibold">{f.title}</h3>
              <p className="mt-2 text-sm text-muted-foreground leading-relaxed">{f.body}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ── Pricing ──────────────────────────────────── */}
      <section className="bg-secondary/20 py-20">
        <div className="container">
          <div className="mx-auto mb-12 max-w-xl text-center">
            <h2 className="text-3xl font-bold tracking-tight">Simple, metered pricing</h2>
            <p className="mt-3 text-muted-foreground">
              Start free. Pay for the seats, storage, and API you actually use.
            </p>
          </div>
          <PricingTable />
        </div>
      </section>

      {/* ── CTA ──────────────────────────────────────── */}
      <section className="relative overflow-hidden py-24">
        {/* Background gradient */}
        <div className="absolute inset-0 -z-10 bg-gradient-to-br from-primary/5 via-accent/5 to-background" />
        <div className="absolute bottom-0 right-1/4 -z-10 h-[300px] w-[500px] rounded-full bg-primary/10 blur-3xl" />

        <div className="container text-center">
          <h2 className="text-3xl font-bold tracking-tight">Ready to get organized?</h2>
          <p className="mt-3 text-muted-foreground">Create your workspace in under a minute.</p>
          <Link href="/register" className="mt-8 inline-block">
            <Button size="lg" className="gap-2 px-10">
              Create your workspace
              <ArrowRight className="h-4 w-4" />
            </Button>
          </Link>
        </div>
      </section>
    </>
  );
}
