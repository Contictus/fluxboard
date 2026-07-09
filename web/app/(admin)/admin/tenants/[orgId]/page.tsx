'use client';

import { useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { UserCog, RefreshCw, Trash2, Plus } from 'lucide-react';

import { useToast } from '@/components/ui/toast';
import { setAccessToken } from '@/lib/api/client';
import {
  deleteOverride,
  getTenantDetail,
  retryWebhook,
  setFlag,
  setOverride,
  startImpersonation,
} from '@/lib/api/admin';
import { formatMoney } from '@/lib/billing/format';
import type { AdminOverride, AdminWebhook } from '@/lib/api/types';

function fmtDate(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function AdminTenantDetailPage() {
  const params = useParams<{ orgId: string }>();
  const orgId = params.orgId;
  const router = useRouter();
  const { toast } = useToast();
  const qc = useQueryClient();

  const detail = useQuery({
    queryKey: ['admin-tenant', orgId],
    queryFn: () => getTenantDetail(orgId),
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin-tenant', orgId] });

  const impersonate = useMutation({
    mutationFn: () => startImpersonation(orgId),
    onSuccess: (res) => {
      // Adopt the read-only impersonation token; the app-wide banner picks up the
      // `imp` claim and offers an exit back to the admin session.
      setAccessToken(res.token);
      const slug = detail.data?.tenant.slug;
      router.push(slug ? `/app/${slug}` : '/app');
    },
    onError: () => toast({ title: 'Couldn’t start impersonation', variant: 'error' }),
  });

  if (detail.isLoading) {
    return <p className="text-sm text-neutral-400">Loading tenant…</p>;
  }
  if (detail.isError || !detail.data) {
    return <p className="text-sm text-red-400">Couldn’t load this tenant.</p>;
  }

  const d = detail.data;
  const t = d.tenant;

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{t.name}</h1>
          <p className="mt-1 text-sm text-neutral-400">
            {t.slug} · {t.org_id}
          </p>
        </div>
        <button
          onClick={() => impersonate.mutate()}
          disabled={impersonate.isPending}
          className="inline-flex h-9 items-center gap-2 rounded-md border border-amber-500/50 px-4 text-sm font-medium text-amber-400 hover:bg-amber-500/10 disabled:opacity-50"
        >
          <UserCog className="h-4 w-4" />
          {impersonate.isPending ? 'Starting…' : 'Impersonate (read-only)'}
        </button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Plan" value={t.plan || 'free'} />
        <Stat label="Status" value={(t.status || 'none').replace('_', ' ')} />
        <Stat label="Members" value={String(t.member_count)} />
        <Stat label="MRR" value={formatMoney(t.mrr)} />
      </div>

      <Section title="Subscription">
        {d.subscription ? (
          <dl className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <Field label="Plan" value={d.subscription.PlanCode} />
            <Field label="Status" value={d.subscription.Status} />
            <Field label="Period end" value={fmtDate(d.subscription.CurrentPeriodEnd)} />
            <Field
              label="Cancel at period end"
              value={d.subscription.CancelAtPeriodEnd ? 'Yes' : 'No'}
            />
          </dl>
        ) : (
          <p className="text-sm text-neutral-500">No active subscription (Free tier).</p>
        )}
      </Section>

      <FlagsSection
        orgId={orgId}
        flags={d.flags}
        onChanged={invalidate}
      />

      <OverridesSection orgId={orgId} overrides={d.overrides} onChanged={invalidate} />

      <WebhooksSection orgId={orgId} webhooks={d.webhooks} onChanged={invalidate} />

      <Section title="Recent invoices">
        {d.invoices.length === 0 ? (
          <p className="text-sm text-neutral-500">No invoices.</p>
        ) : (
          <table className="w-full text-sm">
            <tbody>
              {d.invoices.map((inv) => (
                <tr key={inv.ID} className="border-b border-neutral-800 last:border-0">
                  <td className="py-2 text-neutral-300">{inv.Number || inv.ID}</td>
                  <td className="py-2 capitalize text-neutral-400">{inv.Status}</td>
                  <td className="py-2 text-neutral-400">{fmtDate(inv.CreatedAt)}</td>
                  <td className="py-2 text-right text-neutral-300">
                    {formatMoney(inv.AmountDue, inv.Currency)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-neutral-800 bg-neutral-900 p-4">
      <p className="text-xs font-medium uppercase tracking-wide text-neutral-500">{label}</p>
      <p className="mt-1 text-lg font-semibold capitalize">{value}</p>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-neutral-400">
        {title}
      </h2>
      <div className="rounded-lg border border-neutral-800 bg-neutral-900 p-4">{children}</div>
    </section>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-neutral-500">{label}</dt>
      <dd className="mt-0.5 capitalize text-neutral-200">{value}</dd>
    </div>
  );
}

function FlagsSection({
  orgId,
  flags,
  onChanged,
}: {
  orgId: string;
  flags: { OrgID: string; Flag: string; Enabled: boolean }[];
  onChanged: () => void;
}) {
  const { toast } = useToast();
  const [newFlag, setNewFlag] = useState('');

  const toggle = useMutation({
    mutationFn: ({ flag, enabled }: { flag: string; enabled: boolean }) =>
      setFlag(orgId, flag, enabled),
    onSuccess: onChanged,
    onError: () => toast({ title: 'Couldn’t update flag', variant: 'error' }),
  });

  return (
    <Section title="Feature flags">
      {flags.length === 0 ? (
        <p className="mb-3 text-sm text-neutral-500">No flags set.</p>
      ) : (
        <ul className="mb-4 space-y-2">
          {flags.map((f) => (
            <li key={f.Flag} className="flex items-center justify-between text-sm">
              <span className="font-mono text-neutral-200">{f.Flag}</span>
              <button
                onClick={() => toggle.mutate({ flag: f.Flag, enabled: !f.Enabled })}
                disabled={toggle.isPending}
                className={`rounded-full px-3 py-1 text-xs font-medium ${
                  f.Enabled
                    ? 'bg-emerald-500/20 text-emerald-400'
                    : 'bg-neutral-800 text-neutral-400'
                }`}
              >
                {f.Enabled ? 'Enabled' : 'Disabled'}
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!newFlag.trim()) return;
          toggle.mutate({ flag: newFlag.trim(), enabled: true });
          setNewFlag('');
        }}
      >
        <input
          value={newFlag}
          onChange={(e) => setNewFlag(e.target.value)}
          placeholder="flag_name"
          className="h-9 flex-1 rounded-md border border-neutral-700 bg-neutral-950 px-3 text-sm text-neutral-100 placeholder:text-neutral-600"
        />
        <button
          type="submit"
          className="inline-flex h-9 items-center gap-1 rounded-md bg-neutral-800 px-3 text-sm hover:bg-neutral-700"
        >
          <Plus className="h-4 w-4" /> Enable
        </button>
      </form>
    </Section>
  );
}

function OverridesSection({
  orgId,
  overrides,
  onChanged,
}: {
  orgId: string;
  overrides: AdminOverride[];
  onChanged: () => void;
}) {
  const { toast } = useToast();
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [note, setNote] = useState('');

  const upsert = useMutation({
    mutationFn: () => setOverride(orgId, { key: key.trim(), value: value.trim(), note: note.trim() }),
    onSuccess: () => {
      onChanged();
      setKey('');
      setValue('');
      setNote('');
    },
    onError: () => toast({ title: 'Couldn’t save override', variant: 'error' }),
  });

  const remove = useMutation({
    mutationFn: (k: string) => deleteOverride(orgId, k),
    onSuccess: onChanged,
    onError: () => toast({ title: 'Couldn’t remove override', variant: 'error' }),
  });

  const inputCls =
    'h-9 rounded-md border border-neutral-700 bg-neutral-950 px-3 text-sm text-neutral-100 placeholder:text-neutral-600';

  return (
    <Section title="Entitlement overrides">
      {overrides.length === 0 ? (
        <p className="mb-3 text-sm text-neutral-500">No overrides.</p>
      ) : (
        <ul className="mb-4 space-y-2">
          {overrides.map((o) => (
            <li key={o.Key} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0">
                <span className="font-mono text-neutral-200">{o.Key}</span>
                <span className="text-neutral-400"> = {o.Value}</span>
                {o.Note ? <span className="ml-2 text-xs text-neutral-500">({o.Note})</span> : null}
              </span>
              <button
                onClick={() => remove.mutate(o.Key)}
                disabled={remove.isPending}
                className="shrink-0 rounded-md p-1.5 text-neutral-500 hover:bg-neutral-800 hover:text-red-400"
                aria-label="Remove override"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]"
        onSubmit={(e) => {
          e.preventDefault();
          if (!key.trim() || !value.trim()) return;
          upsert.mutate();
        }}
      >
        <input
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="key (e.g. max_members)"
          className={inputCls}
        />
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="value"
          className={inputCls}
        />
        <input
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="note"
          className={inputCls}
        />
        <button
          type="submit"
          disabled={upsert.isPending}
          className="inline-flex h-9 items-center gap-1 rounded-md bg-neutral-800 px-3 text-sm hover:bg-neutral-700"
        >
          <Plus className="h-4 w-4" /> Set
        </button>
      </form>
    </Section>
  );
}

function WebhooksSection({
  orgId,
  webhooks,
  onChanged,
}: {
  orgId: string;
  webhooks: AdminWebhook[];
  onChanged: () => void;
}) {
  const { toast } = useToast();

  const retry = useMutation({
    mutationFn: (eventId: string) => retryWebhook(orgId, eventId),
    onSuccess: () => {
      onChanged();
      toast({ title: 'Webhook re-enqueued', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t retry webhook', variant: 'error' }),
  });

  return (
    <Section title="Recent webhook events">
      {webhooks.length === 0 ? (
        <p className="text-sm text-neutral-500">No webhook events.</p>
      ) : (
        <table className="w-full text-sm">
          <tbody>
            {webhooks.map((w) => (
              <tr key={w.EventID} className="border-b border-neutral-800 last:border-0">
                <td className="py-2 font-mono text-xs text-neutral-300">{w.Type}</td>
                <td className="py-2">
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs ${
                      w.Handled
                        ? 'bg-emerald-500/20 text-emerald-400'
                        : 'bg-amber-500/20 text-amber-400'
                    }`}
                  >
                    {w.Handled ? 'handled' : 'unhandled'}
                  </span>
                  {w.Error ? (
                    <span className="ml-2 text-xs text-red-400" title={w.Error}>
                      error
                    </span>
                  ) : null}
                </td>
                <td className="py-2 text-neutral-500">{fmtDate(w.ProcessedAt)}</td>
                <td className="py-2 text-right">
                  <button
                    onClick={() => retry.mutate(w.EventID)}
                    disabled={retry.isPending}
                    className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-neutral-300 hover:bg-neutral-800"
                  >
                    <RefreshCw className="h-3.5 w-3.5" /> Retry
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Section>
  );
}
