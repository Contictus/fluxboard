import Link from 'next/link';
import { Activity, ArrowUpRight, CreditCard, FolderKanban, ShieldCheck, Users } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { PricingTable } from '@/components/marketing/pricing-table';

const features = [
  { icon: FolderKanban, title: 'Work that stays visible', body: 'Boards, tasks, and ownership in one calm place.' },
  { icon: Users, title: 'Teams with context', body: 'Keep decisions close to the work without adding process.' },
  { icon: Activity, title: 'A live pulse', body: 'Changes arrive as they happen, so nobody works from stale information.' },
  { icon: CreditCard, title: 'Billing you can trust', body: 'Usage, seats, and invoices are clear from the start.' },
  { icon: ShieldCheck, title: 'Boundaries by default', body: 'Tenant isolation, roles, and audit history built into the foundation.' },
];

export default function LandingPage() {
  return (
    <main className="overflow-hidden">
      <section className="container grid gap-12 pb-24 pt-16 md:grid-cols-[1.1fr_0.9fr] md:items-end md:gap-20 md:pb-32 md:pt-24">
        <div className="max-w-3xl">
          <p className="mb-7 text-sm font-semibold uppercase tracking-[0.18em] text-primary">The operating space for modern teams</p>
          <h1 className="max-w-2xl text-5xl font-semibold leading-[0.98] tracking-[-0.07em] sm:text-6xl lg:text-7xl">Make the work feel lighter.</h1>
          <p className="mt-8 max-w-xl text-lg leading-8 text-muted-foreground">Fluxboard gives growing teams one clear surface for projects, decisions, and the work that moves between them.</p>
          <div className="mt-9 flex flex-wrap items-center gap-3">
            <Link href="/register"><Button size="lg" className="gap-2 px-6">Start for free <ArrowUpRight className="h-4 w-4" /></Button></Link>
            <Link href="/features" className="rounded-xl px-4 py-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground">See how it works</Link>
          </div>
        </div>
        <div className="relative min-h-[300px] rounded-[2rem] bg-[#183a36] p-7 text-[#eef4ee] shadow-2xl shadow-[#183a36]/15 sm:p-9">
          <div className="absolute right-7 top-7 h-16 w-16 rounded-full bg-[#d4e66d]" />
          <div className="relative flex h-full min-h-[245px] flex-col justify-between">
            <div className="flex items-center justify-between text-xs uppercase tracking-[0.16em] text-[#eef4ee]/60"><span>Today</span><span>Flux / 01</span></div>
            <div><p className="max-w-[15ch] text-3xl font-semibold leading-tight tracking-[-0.05em]">A quieter way to move forward.</p><div className="mt-8 flex items-center gap-2 text-sm text-[#eef4ee]/70"><span className="h-2 w-2 rounded-full bg-[#d4e66d]" /> 12 things in motion</div></div>
          </div>
        </div>
      </section>

      <section className="bg-secondary/45 py-20 md:py-28">
        <div className="container">
          <div className="mb-14 max-w-xl"><p className="mb-4 text-sm font-semibold uppercase tracking-[0.18em] text-primary">A better default</p><h2 className="text-4xl font-semibold tracking-[-0.06em] md:text-5xl">Less ceremony. More momentum.</h2></div>
          <div className="grid gap-x-12 gap-y-12 md:grid-cols-2 lg:grid-cols-3">
            {features.map((feature, index) => <div key={feature.title} className={index === 4 ? 'lg:col-span-2' : ''}><feature.icon className="h-5 w-5 text-primary" strokeWidth={1.7} /><h3 className="mt-5 text-xl font-semibold tracking-[-0.03em]">{feature.title}</h3><p className="mt-3 max-w-sm leading-7 text-muted-foreground">{feature.body}</p></div>)}
          </div>
        </div>
      </section>

      <section className="container py-20 md:py-28">
        <div className="mb-14 flex flex-col justify-between gap-5 md:flex-row md:items-end"><div><p className="mb-4 text-sm font-semibold uppercase tracking-[0.18em] text-primary">Simple by design</p><h2 className="text-4xl font-semibold tracking-[-0.06em] md:text-5xl">Pay for the work you do.</h2></div><p className="max-w-sm text-muted-foreground">Start with the essentials. Add capacity when the team needs it.</p></div>
        <PricingTable />
      </section>

      <section className="bg-[#183a36] py-24 text-[#eef4ee] md:py-32"><div className="container grid gap-10 md:grid-cols-[1fr_auto] md:items-end"><div><p className="mb-5 text-sm font-semibold uppercase tracking-[0.18em] text-[#d4e66d]">Ready when you are</p><h2 className="max-w-xl text-4xl font-semibold tracking-[-0.06em] md:text-6xl">Bring the important work closer.</h2></div><Link href="/register"><Button size="lg" className="gap-2 bg-[#d4e66d] text-[#183a36] hover:bg-[#e4f28c]">Create your workspace <ArrowUpRight className="h-4 w-4" /></Button></Link></div></section>
    </main>
  );
}
