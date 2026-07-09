'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useOrg } from '@/lib/org/context';
import { useToast } from '@/components/ui/toast';
import { memberName, useOrgMembers } from '@/lib/org/use-members';
import { auditCsv, listAudit, type AuditFilter } from '@/lib/api/audit';

const SEVERITIES = ['', 'info', 'notice', 'warning', 'critical'] as const;

const SEVERITY_STYLE: Record<string, string> = {
  info: 'bg-secondary text-secondary-foreground',
  notice: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
  warning: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
  critical: 'bg-destructive/15 text-destructive',
};

/** A yyyy-mm-dd date input value → RFC3339 at UTC midnight (empty → undefined). */
function toRfc3339(day: string): string | undefined {
  if (!day) return undefined;
  const d = new Date(`${day}T00:00:00Z`);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

export default function AuditLogSettingsPage() {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const { byId } = useOrgMembers();

  // Draft filter inputs; `applied` is what actually drives the query.
  const [actor, setActor] = useState('');
  const [action, setAction] = useState('');
  const [severity, setSeverity] = useState('');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [applied, setApplied] = useState<AuditFilter>({ limit: 100 });
  const [exporting, setExporting] = useState(false);

  const audit = useQuery({
    queryKey: ['audit', orgId, applied],
    queryFn: () => listAudit(orgId, applied),
  });

  function buildFilter(): AuditFilter {
    return {
      actor: actor.trim() || undefined,
      action: action.trim() || undefined,
      severity: severity || undefined,
      since: toRfc3339(since),
      until: toRfc3339(until),
      limit: 100,
    };
  }

  async function exportCsv() {
    setExporting(true);
    try {
      const csv = await auditCsv(orgId, applied);
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'audit.csv';
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      toast({ title: 'Export failed', variant: 'error' });
    } finally {
      setExporting(false);
    }
  }

  const entries = audit.data ?? [];

  return (
    <div className="max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Audit log</h2>
        <Button size="sm" variant="outline" onClick={exportCsv} disabled={exporting}>
          {exporting ? 'Exporting…' : 'Export CSV'}
        </Button>
      </div>

      <form
        className="flex flex-wrap items-end gap-3 rounded-md border bg-card p-3"
        onSubmit={(e) => {
          e.preventDefault();
          setApplied(buildFilter());
        }}
      >
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Actor ID
          <Input value={actor} onChange={(e) => setActor(e.target.value)} className="h-9 w-40" />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Action
          <Input value={action} onChange={(e) => setAction(e.target.value)} className="h-9 w-40" />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Severity
          <select
            value={severity}
            onChange={(e) => setSeverity(e.target.value)}
            className="h-9 rounded-md border border-input bg-background px-2 text-sm"
          >
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {s ? s.charAt(0).toUpperCase() + s.slice(1) : 'Any'}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Since
          <Input type="date" value={since} onChange={(e) => setSince(e.target.value)} className="h-9" />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Until
          <Input type="date" value={until} onChange={(e) => setUntil(e.target.value)} className="h-9" />
        </label>
        <Button type="submit" size="sm">
          Apply
        </Button>
      </form>

      {audit.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading audit log…</p>
      ) : audit.isError ? (
        <p className="text-sm text-destructive">Couldn’t load the audit log.</p>
      ) : entries.length === 0 ? (
        <p className="rounded-md border bg-secondary/30 p-4 text-sm text-muted-foreground">
          No audit entries match these filters.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="border-b bg-secondary/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="p-2 font-medium">Time</th>
                <th className="p-2 font-medium">Action</th>
                <th className="p-2 font-medium">Severity</th>
                <th className="p-2 font-medium">Actor</th>
                <th className="p-2 font-medium">Target</th>
                <th className="p-2 font-medium">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {entries.map((e, i) => (
                <tr key={`${e.created_at}-${i}`}>
                  <td className="whitespace-nowrap p-2 text-xs text-muted-foreground">
                    {new Date(e.created_at).toLocaleString()}
                  </td>
                  <td className="p-2 font-mono text-xs">{e.action}</td>
                  <td className="p-2">
                    <span
                      className={
                        'rounded-full px-2 py-0.5 text-xs font-medium ' +
                        (SEVERITY_STYLE[e.severity] ?? 'bg-secondary text-secondary-foreground')
                      }
                    >
                      {e.severity}
                    </span>
                  </td>
                  <td className="p-2 text-xs">
                    {e.actor_user_id ? memberName(byId, e.actor_user_id) : '—'}
                    {e.impersonator_user_id ? (
                      <span className="ml-1 text-muted-foreground">(imp)</span>
                    ) : null}
                  </td>
                  <td className="p-2 text-xs text-muted-foreground">
                    {e.target_type ? `${e.target_type}${e.target_id ? ` ${e.target_id.slice(0, 8)}` : ''}` : '—'}
                  </td>
                  <td className="p-2 text-xs text-muted-foreground">{e.ip || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
