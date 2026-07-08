import type { Metadata } from 'next';
import { CheckCircle2 } from 'lucide-react';

export const metadata: Metadata = { title: 'Status' };

const systems = ['API', 'Web app', 'Realtime (SSE)', 'Background jobs', 'Billing'];

export default function StatusPage() {
  return (
    <div className="container max-w-2xl py-16">
      <h1 className="text-4xl font-bold tracking-tight">System status</h1>
      <div className="mt-6 flex items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 p-4">
        <CheckCircle2 className="h-5 w-5 text-primary" />
        <p className="text-sm font-medium">All systems operational</p>
      </div>
      <ul className="mt-8 divide-y rounded-lg border">
        {systems.map((s) => (
          <li key={s} className="flex items-center justify-between p-4 text-sm">
            <span>{s}</span>
            <span className="flex items-center gap-2 text-muted-foreground">
              <span className="h-2 w-2 rounded-full bg-green-500" />
              Operational
            </span>
          </li>
        ))}
      </ul>
      <p className="mt-6 text-sm text-muted-foreground">
        This is a placeholder status page. In production it would link to a dedicated status
        provider with historical uptime.
      </p>
    </div>
  );
}
