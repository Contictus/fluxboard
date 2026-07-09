import { apiFetch, ApiError } from './client';
import type {
  CreateInvitationInput,
  Invitation,
  Org,
  OrgMember,
  OrgSummary,
  Role,
  UpdateOrgInput,
} from './types';

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
 * Edit org profile. `name`/`logo_key` are ADMIN+; a `slug` change is re-gated
 * OWNER-only in the handler (403 otherwise) and 301-redirects the old slug for a
 * window. Returns the updated org (name path) or current org (slug-only path).
 */
export function updateOrg(orgId: string, input: UpdateOrgInput): Promise<Org> {
  return apiFetch(`/orgs/${orgId}`, { method: 'PATCH', body: input });
}

/** Soft-delete the org (204, OWNER). The shell then offers restore within grace. */
export function deleteOrg(orgId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}`, { method: 'DELETE' });
}

/** Move OWNER to another member (204, OWNER only). */
export function transferOwnership(orgId: string, userId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/transfer-ownership`, { method: 'POST', body: { user_id: userId } });
}

// ---- Members (write) -------------------------------------------------------

/** Change a member's role (204, ADMIN+). OWNER is set via transferOwnership, not here. */
export function changeMemberRole(orgId: string, userId: string, role: Role): Promise<void> {
  return apiFetch(`/orgs/${orgId}/members/${userId}`, { method: 'PATCH', body: { role } });
}

/** Remove a member (204, ADMIN+). */
export function removeMember(orgId: string, userId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/members/${userId}`, { method: 'DELETE' });
}

// ---- Invitations -----------------------------------------------------------

/** Pending invitations (ADMIN+). */
export async function listInvitations(orgId: string): Promise<Invitation[]> {
  const res = await apiFetch<{ items: Invitation[] }>(`/orgs/${orgId}/invitations`);
  return res.items;
}

/**
 * Invite one email (ADMIN+). Entitlement-gated — a full seat plan yields 402
 * plan_limit_exceeded. Multi-email UIs call this once per address (no batch API).
 */
export function createInvitation(orgId: string, input: CreateInvitationInput): Promise<Invitation> {
  return apiFetch(`/orgs/${orgId}/invitations`, { method: 'POST', body: input });
}

/** Revoke a pending invitation (204, ADMIN+). */
export function revokeInvitation(orgId: string, id: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/invitations/${id}`, { method: 'DELETE' });
}

/** Rotate + re-send a pending invitation (204, ADMIN+). */
export function resendInvitation(orgId: string, id: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/invitations/${id}/resend`, { method: 'POST' });
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
