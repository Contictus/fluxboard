import type { Metadata } from 'next';

export const metadata: Metadata = { title: 'Changelog' };

interface Entry {
  date: string;
  version: string;
  changes: string[];
}

// Simple content array (no MDX toolchain needed for a flat changelog).
const entries: Entry[] = [
  {
    date: '2026-07-08',
    version: 'Admin & observability',
    changes: [
      'Platform admin panel with tenant management and impersonation.',
      'Org-scoped API keys with read/write scopes.',
      'Prometheus metrics and Grafana dashboards.',
      'OpenAPI 3.1 spec served for client generation.',
    ],
  },
  {
    date: '2026-06-20',
    version: 'Realtime & jobs',
    changes: [
      'Server-sent events for live board updates.',
      'Notification center with per-channel preferences.',
      'Background jobs for email, usage aggregation, and webhook retries.',
    ],
  },
  {
    date: '2026-06-01',
    version: 'Billing',
    changes: [
      'Stripe Checkout, subscriptions, and Billing Portal.',
      'Usage metering for seats, storage, and API calls.',
      'Plan-limit enforcement with upgrade prompts.',
    ],
  },
];

export default function ChangelogPage() {
  return (
    <div className="container max-w-2xl py-16">
      <h1 className="text-4xl font-bold tracking-tight">Changelog</h1>
      <p className="mt-3 text-muted-foreground">Product updates, newest first.</p>
      <div className="mt-10 space-y-10">
        {entries.map((e) => (
          <article key={e.date} className="border-l-2 border-border pl-6">
            <div className="flex items-baseline gap-3">
              <h2 className="text-lg font-semibold">{e.version}</h2>
              <time className="text-sm text-muted-foreground">{e.date}</time>
            </div>
            <ul className="mt-3 list-disc space-y-1 pl-5 text-sm text-muted-foreground">
              {e.changes.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
          </article>
        ))}
      </div>
    </div>
  );
}
