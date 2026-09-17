'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowUpRight, Bell, Check, CheckCheck } from 'lucide-react';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { useToast } from '@/components/ui/toast';
import { useOrg } from '@/lib/org/context';
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
} from '@/lib/api/notifications';
import { getTask } from '@/lib/api/tasks';
import { listProjects } from '@/lib/api/projects';
import { getBoard } from '@/lib/api/board';
import type { Notification } from '@/lib/api/types';

type Tab = 'unread' | 'all';

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.round(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.round(hrs / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

export default function NotificationsPage() {
  const { orgId, slug } = useOrg();
  const router = useRouter();
  const { toast } = useToast();
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>('unread');
  const [openingId, setOpeningId] = useState<string | null>(null);

  const list = useQuery({
    queryKey: ['notifications', orgId, tab],
    queryFn: () => listNotifications(orgId, { unread: tab === 'unread', limit: 50 }),
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['notifications', orgId] });
    qc.invalidateQueries({ queryKey: ['unread-count', orgId] });
  };

  const markRead = useMutation({
    mutationFn: (id: string) => markNotificationRead(orgId, id),
    onSuccess: invalidate,
  });

  const markAll = useMutation({
    mutationFn: () => markAllNotificationsRead(orgId),
    onSuccess: (n) => {
      invalidate();
      toast({ title: n > 0 ? `Marked ${n} as read` : 'All caught up', variant: 'success' });
    },
    onError: () => toast({ title: 'Couldn’t mark all read', variant: 'error' }),
  });

  // Deep-link a task notification to its pretty task URL (ADR-018 follow-up):
  // the row carries only the task UUID, so resolve project + number through
  // the same cached projections the board uses, then navigate.
  async function openNotification(n: Notification) {
    if (n.entity_type !== 'task' || !n.entity_id) return;
    setOpeningId(n.id);
    try {
      const t = await getTask(orgId, n.entity_id);
      const [projects, board] = await Promise.all([
        qc.fetchQuery({ queryKey: ['projects', orgId], queryFn: () => listProjects(orgId) }),
        qc.fetchQuery({
          queryKey: ['board', t.project_id],
          queryFn: () => getBoard(orgId, t.project_id),
        }),
      ]);
      const project = projects.find((p) => p.id === t.project_id);
      const card = board.columns.flatMap((c) => c.tasks).find((c) => c.id === t.id);
      if (!project || !card) throw new Error('not found');
      markRead.mutate(n.id);
      router.push(`/app/${slug}/projects/${project.key}/tasks/${card.number}`);
    } catch {
      toast({ title: 'Couldn’t open that task', variant: 'error' });
    } finally {
      setOpeningId(null);
    }
  }

  const items = list.data ?? [];

  return (
    <div className="mx-auto max-w-3xl px-6 py-10">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">Notifications</h1>
        <Button
          variant="outline"
          size="sm"
          onClick={() => markAll.mutate()}
          disabled={markAll.isPending}
        >
          <CheckCheck className="h-4 w-4" /> Mark all read
        </Button>
      </div>

      <div className="mb-4 flex gap-1 border-b">
        {(['unread', 'all'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={cn(
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium capitalize transition-colors',
              tab === t
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t}
          </button>
        ))}
      </div>

      {list.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center rounded-md border border-dashed py-16 text-center">
          <Bell className="h-8 w-8 text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">
            {tab === 'unread' ? 'No unread notifications.' : 'Nothing here yet.'}
          </p>
        </div>
      ) : (
        <ul className="divide-y rounded-md border">
          {items.map((n) => (
            <NotificationRow
              key={n.id}
              n={n}
              onMarkRead={() => markRead.mutate(n.id)}
              pending={markRead.isPending}
              opening={openingId === n.id}
              onOpen={() => void openNotification(n)}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function NotificationRow({
  n,
  onMarkRead,
  pending,
  opening,
  onOpen,
}: {
  n: Notification;
  onMarkRead: () => void;
  pending: boolean;
  opening: boolean;
  onOpen: () => void;
}) {
  const unread = !n.read_at;
  const linkable = n.entity_type === 'task' && !!n.entity_id;
  return (
    <li className={cn('flex items-start gap-3 p-4', unread && 'bg-secondary/30')}>
      <span
        className={cn(
          'mt-1.5 h-2 w-2 shrink-0 rounded-full',
          unread ? 'bg-primary' : 'bg-transparent',
        )}
        aria-hidden
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{n.title}</p>
        {n.body ? <p className="mt-0.5 text-sm text-muted-foreground">{n.body}</p> : null}
        <p className="mt-1 text-xs text-muted-foreground">
          <span className="capitalize">{n.category.replace(/_/g, ' ')}</span> · {timeAgo(n.created_at)}
        </p>
      </div>
      {linkable ? (
        <button
          onClick={onOpen}
          disabled={opening}
          className="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
          aria-label="Open task"
          title="Open task"
        >
          <ArrowUpRight className="h-4 w-4" />
          {opening ? 'Opening…' : 'Open'}
        </button>
      ) : null}
      {unread ? (
        <button
          onClick={onMarkRead}
          disabled={pending}
          className="shrink-0 rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
          aria-label="Mark as read"
          title="Mark as read"
        >
          <Check className="h-4 w-4" />
        </button>
      ) : null}
    </li>
  );
}
