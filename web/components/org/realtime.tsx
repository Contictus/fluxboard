'use client';

import { useEffect, useRef } from 'react';
import { useQueryClient, type QueryClient } from '@tanstack/react-query';

import { useOrg } from '@/lib/org/context';
import { useAuth } from '@/lib/auth/context';
import { connectSse, type SseEvent } from '@/lib/sse';

// Realtime cache bridge (FR-NTF-001/002, docs/09 §1). Opens the org SSE stream
// and applies *surgical* TanStack invalidations per event — e.g. a task.moved
// touches only that project's board, never a blanket refetch. Events the caller
// produced (actor_id === me) are skipped: the optimistic update already applied.
// Renders nothing.
export function Realtime() {
  const { orgId } = useOrg();
  const { userId, isAuthenticated } = useAuth();
  const qc = useQueryClient();

  // Keep the latest apply-fn in a ref so the stream connects once per org, not
  // on every render.
  const applyRef = useRef<(e: SseEvent) => void>(() => {});
  applyRef.current = (e) => applyEvent(qc, orgId, userId, e);

  useEffect(() => {
    if (!orgId || !isAuthenticated) return;
    const dispose = connectSse(`/orgs/${orgId}/events`, {
      onEvent: (e) => applyRef.current(e),
      onResync: () => {
        // Backlog expired — the safe move is a full refetch of this org's data.
        void qc.invalidateQueries();
      },
    });
    return dispose;
  }, [orgId, isAuthenticated, qc]);

  return null;
}

function applyEvent(
  qc: QueryClient,
  orgId: string,
  me: string | null,
  e: SseEvent,
): void {
  const d = e.data;
  const actor = typeof d.actor_id === 'string' ? d.actor_id : '';
  const isSelf = me != null && actor === me;
  const projectId = typeof d.project_id === 'string' ? d.project_id : undefined;
  const taskId = typeof d.task_id === 'string' ? d.task_id : undefined;

  const inv = (key: readonly unknown[]) => void qc.invalidateQueries({ queryKey: key });

  switch (e.name) {
    case 'task.created':
    case 'task.moved':
    case 'task.deleted':
    case 'task.restored': {
      if (isSelf) return;
      if (projectId) inv(['board', projectId]);
      return;
    }
    case 'task.updated': {
      if (isSelf) return;
      if (projectId) inv(['board', projectId]);
      if (taskId) {
        inv(['task', orgId, taskId]);
        inv(['activity', orgId, taskId]);
      }
      return;
    }
    case 'comment.created': {
      if (isSelf) return;
      if (taskId) {
        inv(['comments', orgId, taskId]);
        inv(['activity', orgId, taskId]);
      }
      return;
    }
    case 'member.joined':
    case 'member.left':
    case 'member.role_changed':
    case 'membership.revoked': {
      inv(['org-members', orgId]);
      inv(['orgs']);
      return;
    }
    case 'notification.created': {
      // Targeted by user_id — only react when it's addressed to me.
      const target = typeof d.user_id === 'string' ? d.user_id : '';
      if (target && me != null && target !== me) return;
      inv(['unread-count', orgId]);
      inv(['notifications', orgId]);
      inv(['home-activity', orgId]);
      return;
    }
    case 'billing.status_changed': {
      inv(['billing-summary', orgId]);
      inv(['usage', orgId]);
      return;
    }
    default:
      return;
  }
}
