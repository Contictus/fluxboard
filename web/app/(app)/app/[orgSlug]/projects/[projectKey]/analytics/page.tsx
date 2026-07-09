'use client';

import { useParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useOrg } from '@/lib/org/context';
import { useProjectByKey } from '@/lib/board/use-project';
import { getProjectAnalytics } from '@/lib/api/analytics';
import type { DailyStat } from '@/lib/api/types';

// A muted palette for the cumulative-flow columns.
const FLOW_COLORS = ['#6366f1', '#22c55e', '#f59e0b', '#ec4899', '#06b6d4', '#a855f7'];

function shortDay(day: string): string {
  const d = new Date(`${day}T00:00:00Z`);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function columnNames(series: DailyStat[]): string[] {
  const set = new Set<string>();
  for (const d of series) for (const k of Object.keys(d.column_snapshot ?? {})) set.add(k);
  return [...set];
}

export default function ProjectAnalyticsPage() {
  const params = useParams<{ projectKey: string }>();
  const { orgId } = useOrg();
  const { project, isLoading: projLoading, isError: projError } = useProjectByKey(params.projectKey);

  const analytics = useQuery({
    queryKey: ['project-analytics', orgId, project?.id],
    queryFn: () => getProjectAnalytics(orgId, project!.id),
    enabled: Boolean(project?.id),
  });

  if (projLoading) return <div className="p-8 text-sm text-muted-foreground">Loading…</div>;
  if (projError || !project) {
    return <div className="p-8 text-sm text-destructive">Project not found.</div>;
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-10">
      <h1 className="text-2xl font-bold tracking-tight">{project.name} · Analytics</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Throughput, work-in-progress and cycle time from the nightly rollups.
      </p>

      {analytics.isLoading ? (
        <p className="mt-8 text-sm text-muted-foreground">Loading analytics…</p>
      ) : analytics.isError || !analytics.data ? (
        <p className="mt-8 text-sm text-destructive">Couldn’t load analytics.</p>
      ) : (
        <AnalyticsBody data={analytics.data} />
      )}
    </div>
  );
}

function AnalyticsBody({
  data,
}: {
  data: {
    total_created: number;
    total_completed: number;
    avg_cycle_seconds?: number;
    series: DailyStat[];
  };
}) {
  const rows = data.series.map((d) => ({
    day: shortDay(d.day),
    created: d.created_count,
    completed: d.completed_count,
    cycleDays: d.avg_cycle_seconds != null ? +(d.avg_cycle_seconds / 86400).toFixed(2) : null,
    ...d.column_snapshot,
  }));
  const cols = columnNames(data.series);
  const avgCycleDays =
    data.avg_cycle_seconds != null ? (data.avg_cycle_seconds / 86400).toFixed(1) : '—';

  return (
    <div className="mt-8 space-y-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Created" value={String(data.total_created)} />
        <Stat label="Completed" value={String(data.total_completed)} />
        <Stat label="Avg cycle time" value={avgCycleDays === '—' ? '—' : `${avgCycleDays}d`} />
      </div>

      <ChartCard title="Throughput (created vs completed)">
        <LineChart data={rows}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis dataKey="day" tick={{ fontSize: 12 }} />
          <YAxis allowDecimals={false} tick={{ fontSize: 12 }} width={28} />
          <Tooltip />
          <Line type="monotone" dataKey="created" stroke="#6366f1" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="completed" stroke="#22c55e" strokeWidth={2} dot={false} />
        </LineChart>
      </ChartCard>

      {cols.length > 0 ? (
        <ChartCard title="Cumulative flow (cards per column)">
          <AreaChart data={rows}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
            <XAxis dataKey="day" tick={{ fontSize: 12 }} />
            <YAxis allowDecimals={false} tick={{ fontSize: 12 }} width={28} />
            <Tooltip />
            {cols.map((c, i) => (
              <Area
                key={c}
                type="monotone"
                dataKey={c}
                stackId="flow"
                stroke={FLOW_COLORS[i % FLOW_COLORS.length]}
                fill={FLOW_COLORS[i % FLOW_COLORS.length]}
                fillOpacity={0.5}
              />
            ))}
          </AreaChart>
        </ChartCard>
      ) : null}

      <ChartCard title="Cycle time (days)">
        <LineChart data={rows}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis dataKey="day" tick={{ fontSize: 12 }} />
          <YAxis tick={{ fontSize: 12 }} width={28} />
          <Tooltip />
          <Line
            type="monotone"
            dataKey="cycleDays"
            stroke="#f59e0b"
            strokeWidth={2}
            dot={false}
            connectNulls
          />
        </LineChart>
      </ChartCard>

      <p className="text-xs text-muted-foreground">
        Per-assignee breakdown isn’t shown — the analytics rollup aggregates by day and column, not
        by assignee (ADR-022).
      </p>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold">{value}</p>
      </CardContent>
    </Card>
  );
}

function ChartCard({ title, children }: { title: string; children: React.ReactElement }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-64 w-full">
          <ResponsiveContainer width="100%" height="100%">
            {children}
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
