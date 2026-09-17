'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link2, X } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { addTaskLink, listTaskLinks, removeTaskLink } from '@/lib/api/links';
import { searchTasks } from '@/lib/api/tasks';
import { useToast } from '@/components/ui/toast';
import { Button } from '@/components/ui/button';

// Dependencies (FR-LINKS): "blocked by" edges with a search-to-link adder.
// Moving a task to the final column fails while open blockers exist.
export function TaskLinks({ taskId, canEdit }: { taskId: string; canEdit: boolean }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const key = ['task-links', taskId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const { data } = useQuery({ queryKey: key, queryFn: () => listTaskLinks(orgId, taskId) });
  const blockers = data?.blockers ?? [];
  const blocked = data?.blocked ?? [];

  const [q, setQ] = useState('');
  const [searching, setSearching] = useState(false);
  const searchQ = useQuery({
    queryKey: ['task-search-links', taskId, q],
    queryFn: () => searchTasks(orgId, { q, limit: 8 }),
    enabled: searching && q.trim().length >= 2,
  });

  const addMutation = useMutation({
    mutationFn: (linkedId: string) => addTaskLink(orgId, taskId, linkedId),
    onSuccess: () => {
      invalidate();
      setQ('');
      setSearching(false);
    },
    onError: () => toast({ title: 'Could not add link', variant: 'error' }),
  });
  const removeMutation = useMutation({
    mutationFn: (linkedId: string) => removeTaskLink(orgId, taskId, linkedId),
    onSuccess: invalidate,
  });

  return (
    <section>
      <h3 className="mb-2 text-sm font-semibold">Dependencies</h3>

      {blockers.length === 0 && blocked.length === 0 ? (
        <p className="text-sm text-muted-foreground">No dependencies. Link blocking tasks below.</p>
      ) : null}

      {blockers.length > 0 ? (
        <div className="mb-2">
          <p className="mb-1 text-xs text-muted-foreground">Blocked by</p>
          <ul className="space-y-1">
            {blockers.map((b) => (
              <li
                key={b.task_id}
                className="flex items-center gap-2 rounded-lg bg-amber-500/10 px-2.5 py-1.5 text-xs"
              >
                <Link2 className="h-3 w-3 text-amber-600 dark:text-amber-400" />
                <span className="font-mono text-muted-foreground">#{b.number}</span>
                <span className="min-w-0 flex-1 truncate font-medium">{b.title}</span>
                {canEdit ? (
                  <button
                    type="button"
                    onClick={() => removeMutation.mutate(b.task_id)}
                    aria-label={`Remove blocker ${b.title}`}
                    className="rounded p-1 text-muted-foreground hover:bg-secondary hover:text-destructive"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {blocked.length > 0 ? (
        <div className="mb-2">
          <p className="mb-1 text-xs text-muted-foreground">Blocking</p>
          <ul className="space-y-1">
            {blocked.map((b) => (
              <li
                key={b.task_id}
                className="flex items-center gap-2 rounded-lg bg-secondary/50 px-2.5 py-1.5 text-xs"
              >
                <Link2 className="h-3 w-3 text-muted-foreground" />
                <span className="font-mono text-muted-foreground">#{b.number}</span>
                <span className="min-w-0 flex-1 truncate font-medium">{b.title}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {canEdit ? (
        searching ? (
          <div className="rounded-xl border p-2">
            <input
              autoFocus
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search tasks to block this one…"
              className="w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none"
            />
            {searchQ.data ? (
              <ul className="mt-1 max-h-40 overflow-y-auto">
                {searchQ.data.tasks
                  .filter((t) => t.id !== taskId)
                  .map((t) => (
                    <li key={t.id}>
                      <button
                        type="button"
                        onClick={() => addMutation.mutate(t.id)}
                        className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs hover:bg-secondary"
                      >
                        <span className="font-mono text-muted-foreground">#{t.number}</span>
                        <span className="truncate">{t.title}</span>
                      </button>
                    </li>
                  ))}
                {searchQ.data.tasks.length === 0 ? (
                  <li className="px-2 py-1.5 text-xs text-muted-foreground">No matches.</li>
                ) : null}
              </ul>
            ) : null}
            <div className="mt-1 flex justify-end">
              <Button size="sm" variant="ghost" onClick={() => setSearching(false)}>
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setSearching(true)}
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            + Link a blocker
          </button>
        )
      ) : null}
    </section>
  );
}
