'use client';

import { useQuery } from '@tanstack/react-query';
import {
  Bar,
  BarChart,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useOrg } from '@/lib/org/context';
import { getUsageDashboard } from '@/lib/api/analytics';
import { getBillingSummary } from '@/lib/api/billing';
import { formatMoney, formatBytes } from '@/lib/billing/format';

// Usage dashboard (FR-AN-002). The /usage endpoint returns current-period
// aggregates (a snapshot), not a daily series — so this shows usage-vs-limit
// meters + the estimated total, not a 30-day time series (ADR-020). Limits come
// from the billing summary's entitlements; -1 = unlimited.
function pct(used: number, limit: number): number | null {
  if (limit < 0) return null; // unlimited
  if (limit === 0) return used > 0 ? 100 : 0;
  return Math.min(100, Math.round((used / limit) * 100));
}

function fmtDate(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

export default function UsagePage() {
  const { orgId } = useOrg();

  const usage = useQuery({ queryKey: ['usage', orgId], queryFn: () => getUsageDashboard(orgId) });
  const summary = useQuery({
    queryKey: ['billing-summary', orgId],
    queryFn: () => getBillingSummary(orgId),
  });

  if (usage.isLoading || summary.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading usage…</p>;
  }
  if (usage.isError || !usage.data || !summary.data) {
    return <p className="text-sm text-destructive">Couldn’t load usage data.</p>;
  }

  const u = usage.data;
  const ent = summary.data.entitlements;

  const meters = [
    { label: 'Seats', used: u.seats, limit: ent.max_members, fmt: (n: number) => String(n) },
    {
      label: 'Storage',
      used: u.storage_bytes,
      limit: ent.max_storage_bytes,
      fmt: formatBytes,
    },
  ];

  const apiChart = [
    { name: 'API calls', value: u.api_calls },
  ];

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">
        Billing period {fmtDate(u.period_start)} – {fmtDate(u.period_end)}.
      </p>

      <div className="grid gap-4 sm:grid-cols-2">
        {meters.map((m) => {
          const p = pct(m.used, m.limit);
          return (
            <Card key={m.label}>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {m.label}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-2xl font-semibold">{m.fmt(m.used)}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  of {m.limit < 0 ? 'unlimited' : m.fmt(m.limit)}
                </p>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-secondary">
                  <div
                    className={`h-full rounded-full ${
                      p !== null && p >= 90 ? 'bg-destructive' : 'bg-primary'
                    }`}
                    style={{ width: `${p ?? 4}%` }}
                  />
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            API calls this period
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-40 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={apiChart} layout="vertical" margin={{ left: 8, right: 16 }}>
                <XAxis type="number" hide />
                <YAxis type="category" dataKey="name" hide />
                <Tooltip
                  cursor={{ fill: 'hsl(var(--secondary))' }}
                  formatter={(v) => [Number(v).toLocaleString(), 'Calls']}
                />
                <Bar dataKey="value" radius={[4, 4, 4, 4]}>
                  <Cell fill="hsl(var(--primary))" />
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <p className="text-2xl font-semibold">{u.api_calls.toLocaleString()}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Estimated total this period
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-3xl font-bold">{formatMoney(u.estimated_total)}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            An estimate based on current usage on the {u.plan} plan. Your final invoice may differ.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
