'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bell, Check, CheckCheck } from 'lucide-react';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { useToast } from '@/components/ui/toast';
import { useOrg } from '@/lib/org/context';
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
} from '@/lib/api/notifications';
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
  const { orgId } = useOrg();
  const { toast } = useToast();
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>('unread');

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
}: {
  n: Notification;
  onMarkRead: () => void;
  pending: boolean;
}) {
  const unread = !n.read_at;
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
