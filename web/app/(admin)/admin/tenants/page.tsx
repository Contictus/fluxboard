'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Search } from 'lucide-react';

import { listTenants } from '@/lib/api/admin';
import { formatMoney } from '@/lib/billing/format';

const PLANS = ['', 'free', 'pro', 'business'];
const STATUSES = ['', 'active', 'trialing', 'past_due', 'unpaid', 'canceled', 'none'];

export default function AdminTenantsPage() {
  const [search, setSearch] = useState('');
  const [plan, setPlan] = useState('');
  const [status, setStatus] = useState('');
  const [applied, setApplied] = useState<{ search: string; plan: string; status: string }>({
    search: '',
    plan: '',
    status: '',
  });

  const tenants = useQuery({
    queryKey: ['admin-tenants', applied],
    queryFn: () => listTenants({ ...applied, limit: 200 }),
  });

  const selectCls =
    'h-9 rounded-md border border-neutral-700 bg-neutral-900 px-2 text-sm text-neutral-100';

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">Tenants</h1>

      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setApplied({ search, plan, status });
        }}
      >
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Name or slug…"
            className="h-9 rounded-md border border-neutral-700 bg-neutral-900 pl-8 pr-3 text-sm text-neutral-100 placeholder:text-neutral-500"
          />
        </div>
        <select value={plan} onChange={(e) => setPlan(e.target.value)} className={selectCls}>
          {PLANS.map((p) => (
            <option key={p} value={p}>
              {p === '' ? 'All plans' : p}
            </option>
          ))}
        </select>
        <select value={status} onChange={(e) => setStatus(e.target.value)} className={selectCls}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s === '' ? 'All statuses' : s}
            </option>
          ))}
        </select>
        <button
          type="submit"
          className="h-9 rounded-md bg-amber-500 px-4 text-sm font-medium text-neutral-950 hover:bg-amber-400"
        >
          Filter
        </button>
      </form>

      {tenants.isLoading ? (
        <p className="text-sm text-neutral-400">Loading…</p>
      ) : tenants.isError ? (
        <p className="text-sm text-red-400">Couldn’t load tenants.</p>
      ) : !tenants.data || tenants.data.length === 0 ? (
        <p className="text-sm text-neutral-400">No tenants match those filters.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-800 text-left text-xs text-neutral-500">
                <th className="px-4 py-2 font-medium">Organization</th>
                <th className="px-4 py-2 font-medium">Plan</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 text-right font-medium">Members</th>
                <th className="px-4 py-2 text-right font-medium">MRR</th>
              </tr>
            </thead>
            <tbody>
              {tenants.data.map((t) => (
                <tr
                  key={t.org_id}
                  className="border-b border-neutral-800 last:border-0 hover:bg-neutral-900"
                >
                  <td className="px-4 py-3">
                    <Link
                      href={`/admin/tenants/${t.org_id}`}
                      className="font-medium text-neutral-100 hover:text-amber-400"
                    >
                      {t.name}
                    </Link>
                    <span className="ml-2 text-xs text-neutral-500">{t.slug}</span>
                  </td>
                  <td className="px-4 py-3 capitalize text-neutral-300">{t.plan || 'free'}</td>
                  <td className="px-4 py-3 capitalize text-neutral-300">
                    {(t.status || 'none').replace('_', ' ')}
                  </td>
                  <td className="px-4 py-3 text-right text-neutral-300">{t.member_count}</td>
                  <td className="px-4 py-3 text-right text-neutral-300">{formatMoney(t.mrr)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
