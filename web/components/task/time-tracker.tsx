'use client';

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pause, Play, Trash2 } from 'lucide-react';

import { useOrg } from '@/lib/org/context';
import { memberName, useOrgMembers } from '@/lib/org/use-members';
import {
  deleteTimeEntry,
  formatDuration,
  listTimeEntries,
  logTime,
  startTimer,
  stopTimer,
  totalSeconds,
} from '@/lib/api/tasks';
import { useToast } from '@/components/ui/toast';
import { Button } from '@/components/ui/button';

function useNowTick(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [active]);
  return now;
}

// Time tracker (FR-TIME): start/stop timer, manual entries, per-task total.
export function TimeTracker({ taskId, canEdit }: { taskId: string; canEdit: boolean }) {
  const { orgId } = useOrg();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { byId } = useOrgMembers();
  const key = ['time', orgId, taskId] as const;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: key });

  const { data: entries = [] } = useQuery({ queryKey: key, queryFn: () => listTimeEntries(orgId, taskId) });
  const running = entries.find((e) => !e.ended_at);
  useNowTick(Boolean(running));

  const [showManual, setShowManual] = useState(false);
  const [mStart, setMStart] = useState('');
  const [mEnd, setMEnd] = useState('');
  const [mNote, setMNote] = useState('');

  const startMutation = useMutation({
    mutationFn: () => startTimer(orgId, taskId),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not start timer', variant: 'error' }),
  });
  const stopMutation = useMutation({
    mutationFn: (id: string) => stopTimer(orgId, taskId, id),
    onSuccess: invalidate,
    onError: () => toast({ title: 'Could not stop timer', variant: 'error' }),
  });
  const logMutation = useMutation({
    mutationFn: () =>
      logTime(orgId, taskId, {
        started_at: new Date(mStart).toISOString(),
        ended_at: new Date(mEnd).toISOString(),
        note: mNote.trim(),
      }),
    onSuccess: () => {
      invalidate();
      setShowManual(false);
      setMStart('');
      setMEnd('');
      setMNote('');
    },
    onError: () => toast({ title: 'Could not save entry', variant: 'error' }),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteTimeEntry(orgId, taskId, id),
    onSuccess: invalidate,
  });

  const total = totalSeconds(entries);

  return (
    <section>
      <div className="mb-2 flex items-center gap-2">
        <h3 className="text-sm font-semibold">Time tracking</h3>
        <span className="rounded-full bg-secondary px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
          {formatDuration(total)}
        </span>
        {canEdit ? (
          running ? (
            <Button
              size="sm"
              variant="outline"
              disabled={stopMutation.isPending}
              onClick={() => stopMutation.mutate(running.id)}
              aria-label="Stop timer"
            >
              <Pause className="h-3.5 w-3.5" />
              {formatDuration(Math.floor((Date.now() - new Date(running.started_at).getTime()) / 1000))}
            </Button>
          ) : (
            <Button
              size="sm"
              variant="outline"
              disabled={startMutation.isPending}
              onClick={() => startMutation.mutate()}
              aria-label="Start timer"
            >
              <Play className="h-3.5 w-3.5" /> Start
            </Button>
          )
        ) : null}
      </div>

      {entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">No time logged yet.</p>
      ) : (
        <ul className="space-y-1.5">
          {entries.map((e) => (
            <li
              key={e.id}
              className="flex items-center gap-2 rounded-lg bg-secondary/50 px-2.5 py-1.5 text-xs"
            >
              {!e.ended_at ? (
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-green-500" aria-label="Running" />
              ) : null}
              <span className="font-medium">{memberName(byId, e.user_id)}</span>
              <span className="text-muted-foreground">
                {new Date(e.started_at).toLocaleString(undefined, {
                  month: 'short',
                  day: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                })}
                {e.ended_at
                  ? ` → ${new Date(e.ended_at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}`
                  : ' → now'}
              </span>
              {e.note ? <span className="truncate text-muted-foreground">· {e.note}</span> : null}
              <span className="ml-auto font-mono text-muted-foreground">{formatDuration(e.seconds)}</span>
              {canEdit && e.ended_at ? (
                <button
                  type="button"
                  onClick={() => deleteMutation.mutate(e.id)}
                  aria-label="Delete entry"
                  className="rounded p-1 text-muted-foreground hover:bg-secondary hover:text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      {canEdit ? (
        showManual ? (
          <div className="mt-2 space-y-2 rounded-xl border p-3">
            <div className="grid grid-cols-2 gap-2">
              <label className="text-xs text-muted-foreground">
                Start
                <input
                  type="datetime-local"
                  value={mStart}
                  onChange={(e) => setMStart(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none"
                />
              </label>
              <label className="text-xs text-muted-foreground">
                End
                <input
                  type="datetime-local"
                  value={mEnd}
                  onChange={(e) => setMEnd(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none"
                />
              </label>
            </div>
            <input
              value={mNote}
              onChange={(e) => setMNote(e.target.value)}
              placeholder="Note (optional)"
              maxLength={500}
              className="w-full rounded-lg border border-input bg-background px-2 py-1.5 text-xs outline-none"
            />
            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={!mStart || !mEnd || logMutation.isPending}
                onClick={() => logMutation.mutate()}
              >
                {logMutation.isPending ? 'Saving…' : 'Log time'}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setShowManual(false)}>
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setShowManual(true)}
            className="mt-2 text-xs text-muted-foreground hover:text-foreground"
          >
            + Log time manually
          </button>
        )
      ) : null}
    </section>
  );
}
