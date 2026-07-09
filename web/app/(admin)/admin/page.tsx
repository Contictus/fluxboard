'use client';

import { useQuery } from '@tanstack/react-query';
import { Building2, Users, DollarSign, Activity } from 'lucide-react';

import { listTenants } from '@/lib/api/admin';
import { formatMoney } from '@/lib/billing/format';
import type { AdminTenant } from '@/lib/api/types';

// Platform KPI dashboard (FR-ADM-001). There is no dedicated KPI endpoint, so the
// headline metrics are derived client-side from the tenant list (ADR-022).
export default function AdminDashboardPage() {
  const tenants = useQuery({
    queryKey: ['admin-tenants', {}],
    queryFn: () => listTenants({ limit: 500 }),
  });

  if (tenants.isLoading) {
    return <p className="text-sm text-neutral-400">Loading platform metrics…</p>;
  }
  if (tenants.isError || !tenants.data) {
    return <p className="text-sm text-red-400">Couldn’t load platform metrics.</p>;
  }

  const list = tenants.data;
  const totalMrr = list.reduce((sum, t) => sum + t.mrr, 0);
  const totalMembers = list.reduce((sum, t) => sum + t.member_count, 0);
  const paying = list.filter((t) => t.mrr > 0).length;

  const byPlan = groupCount(list, (t) => t.plan || 'free');
  const byStatus = groupCount(list, (t) => t.status || 'none');

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold tracking-tight">Dashboard</h1>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi icon={Building2} label="Organizations" value={list.length.toLocaleString()} />
        <Kpi icon={DollarSign} label="Total MRR" value={formatMoney(totalMrr)} />
        <Kpi icon={Activity} label="Paying orgs" value={paying.toLocaleString()} />
        <Kpi icon={Users} label="Total members" value={totalMembers.toLocaleString()} />
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Breakdown title="By plan" rows={byPlan} total={list.length} />
        <Breakdown title="By subscription status" rows={byStatus} total={list.length} />
      </div>
    </div>
  );
}

function Kpi({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Building2;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-lg border border-neutral-800 bg-neutral-900 p-4">
      <div className="flex items-center gap-2 text-neutral-400">
        <Icon className="h-4 w-4" />
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </div>
  );
}

function groupCount(list: AdminTenant[], key: (t: AdminTenant) => string): [string, number][] {
  const m = new Map<string, number>();
  for (const t of list) m.set(key(t), (m.get(key(t)) ?? 0) + 1);
  return [...m.entries()].sort((a, b) => b[1] - a[1]);
}

function Breakdown({
  title,
  rows,
  total,
}: {
  title: string;
  rows: [string, number][];
  total: number;
}) {
  return (
    <div className="rounded-lg border border-neutral-800 bg-neutral-900 p-5">
      <h2 className="mb-4 text-sm font-semibold text-neutral-300">{title}</h2>
      <ul className="space-y-3">
        {rows.map(([label, count]) => {
          const pct = total > 0 ? Math.round((count / total) * 100) : 0;
          return (
            <li key={label}>
              <div className="mb-1 flex items-center justify-between text-sm">
                <span className="capitalize text-neutral-300">{label.replace(/_/g, ' ')}</span>
                <span className="text-neutral-400">
                  {count} · {pct}%
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-neutral-800">
                <div className="h-full rounded-full bg-amber-400" style={{ width: `${pct}%` }} />
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
