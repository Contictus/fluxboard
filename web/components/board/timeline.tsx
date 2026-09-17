'use client';

import Link from 'next/link';
import { useMemo } from 'react';

import type { TaskCard } from '@/lib/api/types';
import { cn } from '@/lib/utils';

const DAY = 24 * 60 * 60 * 1000;
const DAY_PX = 28;
const WEEKS = 14;
const PAST_WEEKS = 2;

function startOfDay(d: Date): Date {
  const c = new Date(d);
  c.setHours(0, 0, 0, 0);
  return c;
}

function startOfWeek(d: Date): Date {
  const c = startOfDay(d);
  const dow = (c.getDay() + 6) % 7; // Monday-first
  c.setTime(c.getTime() - dow * DAY);
  return c;
}

// Project timeline (Gantt-style): one row per scheduled task, bars from
// creation to due date, overdue highlighted, unscheduled tasks grouped below.
export function Timeline({
  tasks,
  taskHref,
}: {
  tasks: { task: TaskCard; columnName: string }[];
  taskHref: (task: TaskCard) => string;
}) {
  const windowStart = useMemo(() => {
    const w = startOfWeek(new Date());
    w.setTime(w.getTime() - PAST_WEEKS * 7 * DAY);
    return w;
  }, []);
  const totalDays = WEEKS * 7;

  const todayIdx = Math.floor((startOfDay(new Date()).getTime() - windowStart.getTime()) / DAY);

  const scheduled = useMemo(() => {
    const rows: { task: TaskCard; columnName: string; from: number; to: number; overdue: boolean }[] = [];
    for (const row of tasks) {
      if (!row.task.due_date) continue;
      const due = startOfDay(new Date(row.task.due_date));
      const from = Math.max(
        0,
        Math.floor((startOfDay(new Date(row.task.created_at)).getTime() - windowStart.getTime()) / DAY),
      );
      const to = Math.floor((due.getTime() - windowStart.getTime()) / DAY);
      if (to < 0) continue; // entirely before the window
      rows.push({
        ...row,
        from,
        to: Math.min(to, totalDays - 1),
        overdue: due.getTime() < startOfDay(new Date()).getTime(),
      });
    }
    rows.sort((a, b) => a.to - b.to);
    return rows;
  }, [tasks, windowStart, totalDays]);

  const unscheduled = useMemo(() => tasks.filter((r) => !r.task.due_date), [tasks]);

  const days = useMemo(() => {
    const out: Date[] = [];
    for (let i = 0; i < totalDays; i++) {
      const d = new Date(windowStart);
      d.setTime(d.getTime() + i * DAY);
      out.push(d);
    }
    return out;
  }, [windowStart, totalDays]);

  // Month label spans for the header.
  const months = useMemo(() => {
    const spans: { label: string; days: number }[] = [];
    for (const d of days) {
      const label = d.toLocaleDateString(undefined, { month: 'short' });
      const last = spans[spans.length - 1];
      if (last && days[0] && last.label === label) last.days += 1;
      else spans.push({ label, days: 1 });
    }
    return spans;
  }, [days]);

  if (tasks.length === 0) {
    return <p className="text-sm text-muted-foreground">No tasks yet. Add tasks on the board to see them here.</p>;
  }

  return (
    <div className="space-y-8">
      {scheduled.length > 0 ? (
        <div className="scrollbar-thin overflow-x-auto rounded-2xl border bg-card">
          <div style={{ minWidth: 240 + totalDays * DAY_PX }}>
            <div className="flex border-b">
              <div className="w-60 shrink-0 p-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Task
              </div>
              <div className="flex flex-1">
                {months.map((m, i) => (
                  <div
                    key={i}
                    style={{ width: m.days * DAY_PX }}
                    className="shrink-0 border-l px-2 py-1.5 text-xs font-medium text-muted-foreground"
                  >
                    {m.label}
                  </div>
                ))}
              </div>
            </div>
            <div className="relative">
              {todayIdx >= 0 && todayIdx < totalDays ? (
                <div
                  className="pointer-events-none absolute bottom-0 top-0 z-10 w-px bg-red-500/70"
                  style={{ left: 240 + todayIdx * DAY_PX + DAY_PX / 2 }}
                />
              ) : null}
              {scheduled.map(({ task, columnName, from, to, overdue }) => (
                <div key={task.id} className="flex items-center border-b last:border-0">
                  <div className="w-60 shrink-0 truncate p-3">
                    <Link
                      href={taskHref(task)}
                      className="truncate text-sm font-medium hover:text-primary"
                      title={`${task.title} · ${columnName}`}
                    >
                      <span className="mr-1.5 font-mono text-[11px] text-muted-foreground">#{task.number}</span>
                      {task.title}
                    </Link>
                  </div>
                  <div className="relative h-9 flex-1">
                    <Link
                      href={taskHref(task)}
                      title={`${task.title} — due ${new Date(task.due_date!).toLocaleDateString()}`}
                      className={cn(
                        'absolute top-1/2 h-4 -translate-y-1/2 rounded-full transition-opacity hover:opacity-80',
                        overdue ? 'bg-red-500/80' : 'bg-primary/80',
                      )}
                      style={{ left: from * DAY_PX + 2, width: Math.max((to - from + 1) * DAY_PX - 4, 8) }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">No scheduled tasks yet — set due dates to plan on the timeline.</p>
      )}

      {unscheduled.length > 0 ? (
        <div>
          <h3 className="mb-2 text-sm font-semibold text-muted-foreground">
            Unscheduled ({unscheduled.length})
          </h3>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {unscheduled.map(({ task, columnName }) => (
              <Link
                key={task.id}
                href={taskHref(task)}
                className="truncate rounded-xl border bg-card px-3 py-2 text-sm hover:border-primary/50"
                title={`${task.title} · ${columnName}`}
              >
                <span className="mr-1.5 font-mono text-[11px] text-muted-foreground">#{task.number}</span>
                {task.title}
              </Link>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
