'use client';

import { useQuery } from '@tanstack/react-query';

import { getJobs } from '@/lib/api/admin';

// Asynq queue summary (FR-ADM-005). The endpoint returns an inspector-shaped map
// (or { available: false } when the inspector is disabled); we render it
// generically since the exact shape depends on the queues configured.
export default function AdminJobsPage() {
  const jobs = useQuery({ queryKey: ['admin-jobs'], queryFn: getJobs, refetchInterval: 10_000 });

  if (jobs.isLoading) {
    return <p className="text-sm text-neutral-400">Loading queue status…</p>;
  }
  if (jobs.isError || !jobs.data) {
    return <p className="text-sm text-red-400">Couldn’t load queue status.</p>;
  }

  const data = jobs.data;

  if (data.available === false) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold tracking-tight">Background jobs</h1>
        <p className="rounded-md border border-neutral-800 bg-neutral-900 p-4 text-sm text-neutral-400">
          The Asynq inspector is not enabled on this deployment.
        </p>
      </div>
    );
  }

  const entries = Object.entries(data);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">Background jobs</h1>
      <p className="text-xs text-neutral-500">Auto-refreshes every 10 seconds.</p>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {entries.map(([key, value]) => (
          <div key={key} className="rounded-lg border border-neutral-800 bg-neutral-900 p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-neutral-500">
              {key.replace(/_/g, ' ')}
            </p>
            <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-words text-sm text-neutral-200">
              {typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)}
            </pre>
          </div>
        ))}
      </div>
    </div>
  );
}
