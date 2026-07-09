import { apiFetch } from './client';
import type { Notification, NotificationPref } from './types';

// Per-org notification center + preferences (docs/09 §3, FR-NTF-002/004). Prefs
// are scoped to (org, user, category) by design — there is no global store
// (ADR-015), so the account page composes these per selected org.

/** Caller's own notifications, newest-first (org-home "recent activity", ADR-016). */
export async function listNotifications(
  orgId: string,
  opts: { unread?: boolean; limit?: number } = {},
): Promise<Notification[]> {
  const qs = new URLSearchParams();
  if (opts.unread) qs.set('unread', '1');
  if (opts.limit) qs.set('limit', String(opts.limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  const res = await apiFetch<{ notifications: Notification[] }>(
    `/orgs/${orgId}/notifications${suffix}`,
  );
  return res.notifications;
}

/** Unread badge count for the topbar bell. */
export async function getUnreadCount(orgId: string): Promise<number> {
  const res = await apiFetch<{ unread: number }>(`/orgs/${orgId}/notifications/unread-count`);
  return res.unread;
}

export async function getPrefs(orgId: string): Promise<NotificationPref[]> {
  const res = await apiFetch<{ prefs: NotificationPref[] }>(
    `/orgs/${orgId}/notifications/prefs`,
  );
  return res.prefs;
}

export function setPref(orgId: string, pref: NotificationPref): Promise<void> {
  return apiFetch(`/orgs/${orgId}/notifications/prefs`, { method: 'PUT', body: pref });
}

/** User-configurable categories (transactional auth categories are excluded). */
export const NOTIFICATION_CATEGORIES: { key: string; label: string }[] = [
  { key: 'task_assigned', label: 'Task assigned to me' },
  { key: 'mention', label: 'Someone @mentions me' },
  { key: 'comment', label: 'New comment on my tasks' },
  { key: 'invite_accepted', label: 'Invitation accepted' },
  { key: 'billing', label: 'Billing & usage' },
];
