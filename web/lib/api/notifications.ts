import { apiFetch } from './client';
import type { NotificationPref } from './types';

// Per-org notification preferences (docs/09 §3, FR-NTF-004). Prefs are scoped to
// (org, user, category) by design — there is no global store (ADR-015), so the
// account page composes these per selected org.

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
