import { apiFetch, ApiError } from './client';
import type { Org, OrgSummary } from './types';

// Organizations surface (docs/08 §4). Only the endpoints §3 needs are wrapped here.

export async function listMyOrgs(): Promise<OrgSummary[]> {
  const res = await apiFetch<{ items: OrgSummary[] }>('/orgs');
  return res.items;
}

export function createOrg(input: { name: string; slug: string }): Promise<Org> {
  return apiFetch('/orgs', { method: 'POST', body: input });
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
