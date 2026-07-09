'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { globalAudit } from '@/lib/api/admin';
import type { AuditFilter } from '@/lib/api/audit';

const SEVERITIES = ['', 'info', 'notice', 'warning', 'critical'];

const SEVERITY_STYLE: Record<string, string> = {
  info: 'bg-neutral-800 text-neutral-300',
  notice: 'bg-sky-500/20 text-sky-400',
  warning: 'bg-amber-500/20 text-amber-400',
  critical: 'bg-red-500/20 text-red-400',
};

function toRfc3339(day: string): string | undefined {
  if (!day) return undefined;
  return new Date(`${day}T00:00:00Z`).toISOString();
}

function fmt(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function AdminAuditLogPage() {
  const [actor, setActor] = useState('');
  const [action, setAction] = useState('');
  const [severity, setSeverity] = useState('');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [applied, setApplied] = useState<AuditFilter>({ limit: 100 });

  const audit = useQuery({
    queryKey: ['admin-audit', applied],
    queryFn: () => globalAudit(applied),
  });

  const inputCls =
    'h-9 rounded-md border border-neutral-700 bg-neutral-900 px-3 text-sm text-neutral-100 placeholder:text-neutral-600';

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">Global audit log</h1>

      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setApplied({
            actor: actor || undefined,
            action: action || undefined,
            severity: severity || undefined,
            since: toRfc3339(since),
            until: toRfc3339(until),
            limit: 100,
          });
        }}
      >
        <input value={actor} onChange={(e) => setActor(e.target.value)} placeholder="Actor id" className={inputCls} />
        <input value={action} onChange={(e) => setAction(e.target.value)} placeholder="Action" className={inputCls} />
        <select value={severity} onChange={(e) => setSeverity(e.target.value)} className={inputCls}>
          {SEVERITIES.map((s) => (
            <option key={s} value={s}>
              {s === '' ? 'Any severity' : s}
            </option>
          ))}
        </select>
        <input type="date" value={since} onChange={(e) => setSince(e.target.value)} className={inputCls} />
        <input type="date" value={until} onChange={(e) => setUntil(e.target.value)} className={inputCls} />
        <button
          type="submit"
          className="h-9 rounded-md bg-amber-500 px-4 text-sm font-medium text-neutral-950 hover:bg-amber-400"
        >
          Filter
        </button>
      </form>

      {audit.isLoading ? (
        <p className="text-sm text-neutral-400">Loading…</p>
      ) : audit.isError ? (
        <p className="text-sm text-red-400">Couldn’t load the audit log.</p>
      ) : !audit.data || audit.data.length === 0 ? (
        <p className="text-sm text-neutral-400">No entries match those filters.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-800 text-left text-xs text-neutral-500">
                <th className="px-4 py-2 font-medium">Time</th>
                <th className="px-4 py-2 font-medium">Action</th>
                <th className="px-4 py-2 font-medium">Severity</th>
                <th className="px-4 py-2 font-medium">Actor</th>
                <th className="px-4 py-2 font-medium">Org</th>
                <th className="px-4 py-2 font-medium">Target</th>
              </tr>
            </thead>
            <tbody>
              {audit.data.map((e, i) => (
                <tr key={i} className="border-b border-neutral-800 last:border-0">
                  <td className="whitespace-nowrap px-4 py-2 text-neutral-400">{fmt(e.created_at)}</td>
                  <td className="px-4 py-2 font-mono text-xs text-neutral-200">{e.action}</td>
                  <td className="px-4 py-2">
                    <span
                      className={`rounded-full px-2 py-0.5 text-xs ${
                        SEVERITY_STYLE[e.severity] ?? 'bg-neutral-800 text-neutral-300'
                      }`}
                    >
                      {e.severity}
                    </span>
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-neutral-400">
                    {e.actor_user_id ? e.actor_user_id.slice(0, 8) : '—'}
                    {e.impersonator_user_id ? (
                      <span className="ml-1 text-amber-400" title={`impersonated by ${e.impersonator_user_id}`}>
                        (imp)
                      </span>
                    ) : null}
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-neutral-400">
                    {e.org_id ? e.org_id.slice(0, 8) : '—'}
                  </td>
                  <td className="px-4 py-2 text-xs text-neutral-400">
                    {e.target_type ? `${e.target_type}${e.target_id ? ` ${e.target_id.slice(0, 8)}` : ''}` : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
