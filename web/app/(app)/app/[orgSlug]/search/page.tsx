'use client';

import { Suspense, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { Search as SearchIcon } from 'lucide-react';

import { searchTasks } from '@/lib/api/tasks';
import { listLabels } from '@/lib/api/labels';
import { useOrg } from '@/lib/org/context';
import { useOrgMembers } from '@/lib/org/use-members';
import { useProjectsMap } from '@/lib/board/use-projects';
import { PRIORITIES, priorityLabel } from '@/lib/board/priority';
import { TaskRow } from '@/components/task/task-row';
import type { Priority } from '@/lib/api/types';

// Full-text task search with a filter sidebar (7.6.5, FR-TASK-007). `q` lives in
// the URL so results are shareable/bookmarkable; the facet filters are local
// state. Every filter maps to a server query param.
export default function SearchPage() {
  // useSearchParams requires a Suspense boundary during prerender (Next 14).
  return (
    <Suspense fallback={<p className="px-6 py-8 text-sm text-muted-foreground">Loading…</p>}>
      <SearchInner />
    </Suspense>
  );
}

function SearchInner() {
  const { orgId, slug } = useOrg();
  const router = useRouter();
  const searchParams = useSearchParams();
  const q = searchParams.get('q') ?? '';

  const { projects, byId } = useProjectsMap();
  const { members } = useOrgMembers();
  const { data: labels } = useQuery({ queryKey: ['labels', orgId], queryFn: () => listLabels(orgId) });

  const [projectId, setProjectId] = useState('');
  const [assigneeId, setAssigneeId] = useState('');
  const [priority, setPriority] = useState('');
  const [labelId, setLabelId] = useState('');
  const [input, setInput] = useState(q);

  const filters = { q, project_id: projectId, assignee_id: assigneeId, priority, label_id: labelId };
  const enabled = Boolean(q || projectId || assigneeId || priority || labelId);

  const { data, isLoading, isError } = useQuery({
    queryKey: ['search', orgId, filters],
    queryFn: () => searchTasks(orgId, { ...filters, limit: 50 }),
    enabled,
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const next = new URLSearchParams(searchParams.toString());
    if (input.trim()) next.set('q', input.trim());
    else next.delete('q');
    router.replace(`/app/${slug}/search?${next.toString()}`);
  }

  const results = data?.tasks ?? [];

  return (
    <div className="px-6 py-6">
      <h1 className="mb-4 text-xl font-bold tracking-tight">Search</h1>

      <form onSubmit={submit} className="mb-6 flex gap-2">
        <div className="relative flex-1">
          <SearchIcon className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Search tasks…"
            className="w-full rounded-md border border-input bg-background py-2 pl-9 pr-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <button type="submit" className="rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground">
          Search
        </button>
      </form>

      <div className="grid gap-6 lg:grid-cols-[14rem_1fr]">
        <aside className="space-y-4">
          <Facet label="Project">
            <select value={projectId} onChange={(e) => setProjectId(e.target.value)} className={selectCls}>
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </Facet>
          <Facet label="Assignee">
            <select value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)} className={selectCls}>
              <option value="">Anyone</option>
              {members.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.name || m.email}
                </option>
              ))}
            </select>
          </Facet>
          <Facet label="Priority">
            <select value={priority} onChange={(e) => setPriority(e.target.value)} className={selectCls}>
              <option value="">Any</option>
              {PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {priorityLabel(p as Priority)}
                </option>
              ))}
            </select>
          </Facet>
          <Facet label="Label">
            <select value={labelId} onChange={(e) => setLabelId(e.target.value)} className={selectCls}>
              <option value="">Any</option>
              {(labels ?? []).map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name}
                </option>
              ))}
            </select>
          </Facet>
        </aside>

        <div>
          {!enabled ? (
            <p className="text-sm text-muted-foreground">Type a query or pick a filter to search.</p>
          ) : isLoading ? (
            <p className="text-sm text-muted-foreground">Searching…</p>
          ) : isError ? (
            <p className="text-sm text-destructive">Search failed.</p>
          ) : results.length === 0 ? (
            <p className="text-sm text-muted-foreground">No matching tasks.</p>
          ) : (
            <>
              <p className="mb-2 text-xs text-muted-foreground">
                {data?.total ?? results.length} result{(data?.total ?? results.length) === 1 ? '' : 's'}
              </p>
              <div className="space-y-2">
                {results.map((t) => (
                  <TaskRow key={t.id} task={t} project={byId.get(t.project_id)} orgSlug={slug} />
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

const selectCls =
  'w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

function Facet({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
      {children}
    </div>
  );
}
