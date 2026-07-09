import { apiFetch, ApiError } from './client';
import type { Org, OrgMember, OrgSummary } from './types';

// Organizations surface (docs/08 §4). Only the endpoints §3 needs are wrapped here.

export async function listMyOrgs(): Promise<OrgSummary[]> {
  const res = await apiFetch<{ items: OrgSummary[] }>('/orgs');
  return res.items;
}

/**
 * Org members (keyset-paginated). Used for assignee pickers, @mention
 * autocomplete and rendering user ids as names across the task surface. Callers
 * that need the full set page via `next_cursor`; a single default page (server
 * cap) covers small orgs — the picker/mention UIs also filter by `q`.
 */
export async function listOrgMembers(
  orgId: string,
  params: { q?: string; role?: string; cursor?: string; limit?: number } = {},
): Promise<{ items: OrgMember[]; next_cursor: string }> {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') qs.set(k, String(v));
  }
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return apiFetch(`/orgs/${orgId}/members${suffix}`);
}

export function createOrg(input: { name: string; slug: string }): Promise<Org> {
  return apiFetch('/orgs', { method: 'POST', body: input });
}

/** Full org by id — carries `deleted_at`/`logo_key` the membership list omits. */
export function getOrg(orgId: string): Promise<Org> {
  return apiFetch(`/orgs/${orgId}`);
}

/** Restore a soft-deleted org (204, OWNER-only). */
export function restoreOrg(orgId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/restore`, { method: 'POST' });
}

/**
 * Slug availability check. GET /orgs/by-slug/{slug} resolves an existing org, so a
 * 404 means the slug is free; a 200 means it's taken.
 */
export async function isSlugAvailable(slug: string): Promise<boolean> {
  try {
    await apiFetch(`/orgs/by-slug/${encodeURIComponent(slug)}`);
    return false;
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return true;
    throw e;
  }
}
