'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';

import type { TaskCard } from '@/lib/api/types';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

function startOfDay(d: Date): Date {
  const c = new Date(d);
  c.setHours(0, 0, 0, 0);
  return c;
}

function monthCells(year: number, month: number): Date[] {
  const first = new Date(year, month, 1);
  const lead = (first.getDay() + 6) % 7; // Monday-first
  const cells: Date[] = [];
  for (let i = 0; i < 42; i++) {
    cells.push(new Date(year, month, 1 - lead + i));
  }
  return cells;
}

// Month calendar of due dates: tasks land on their due day, clicking a task
// opens it, clicking an empty day starts a quick task due that day.
export function TaskCalendar({
  tasks,
  taskHref,
  onQuickAdd,
}: {
  tasks: TaskCard[];
  taskHref: (task: TaskCard) => string;
  onQuickAdd: (dueISO: string) => void;
}) {
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth());

  const cells = useMemo(() => monthCells(year, month), [year, month]);
  const today = startOfDay(new Date()).getTime();

  const byDay = useMemo(() => {
    const map = new Map<number, TaskCard[]>();
    for (const t of tasks) {
      if (!t.due_date) continue;
      const key = startOfDay(new Date(t.due_date)).getTime();
      const list = map.get(key) ?? [];
      list.push(t);
      map.set(key, list);
    }
    return map;
  }, [tasks]);

  function step(dir: number) {
    const d = new Date(year, month + dir, 1);
    setYear(d.getFullYear());
    setMonth(d.getMonth());
  }

  const title = new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });

  return (
    <div>
      <div className="mb-4 flex items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => step(-1)} aria-label="Previous month">
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <h2 className="min-w-44 text-center text-lg font-semibold">{title}</h2>
        <Button size="sm" variant="outline" onClick={() => step(1)} aria-label="Next month">
          <ChevronRight className="h-4 w-4" />
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            const n = new Date();
            setYear(n.getFullYear());
            setMonth(n.getMonth());
          }}
        >
          Today
        </Button>
      </div>

      <div className="overflow-hidden rounded-2xl border bg-card">
        <div className="grid grid-cols-7 border-b">
          {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((d) => (
            <div key={d} className="px-2 py-2 text-center text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {d}
            </div>
          ))}
        </div>
        <div className="grid grid-cols-7">
          {cells.map((d, i) => {
            const key = startOfDay(d).getTime();
            const dayTasks = byDay.get(key) ?? [];
            const inMonth = d.getMonth() === month;
            const isToday = key === today;
            return (
              <button
                key={i}
                type="button"
                onClick={() => onQuickAdd(new Date(key).toISOString())}
                title="Add task due this day"
                className={cn(
                  'min-h-24 border-b border-r p-1.5 text-left align-top transition-colors last:[&:nth-child(7n)]:border-r-0 hover:bg-secondary/40 [&:nth-child(7n)]:border-r-0',
                  !inMonth && 'bg-secondary/30 text-muted-foreground',
                )}
              >
                <span
                  className={cn(
                    'inline-flex h-6 w-6 items-center justify-center rounded-full text-xs',
                    isToday ? 'bg-primary font-semibold text-primary-foreground' : 'text-muted-foreground',
                  )}
                >
                  {d.getDate()}
                </span>
                <div className="mt-1 space-y-1">
                  {dayTasks.slice(0, 3).map((t) => (
                    <Link
                      key={t.id}
                      href={taskHref(t)}
                      onClick={(e) => e.stopPropagation()}
                      title={t.title}
                      className="block truncate rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] font-medium text-primary hover:bg-primary/20"
                    >
                      {t.title}
                    </Link>
                  ))}
                  {dayTasks.length > 3 ? (
                    <span className="block px-1.5 text-[11px] text-muted-foreground">
                      +{dayTasks.length - 3} more
                    </span>
                  ) : null}
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
