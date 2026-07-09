'use client';

import { useQuery } from '@tanstack/react-query';

import { listOrgMembers } from '@/lib/api/orgs';
import { useOrg } from '@/lib/org/context';
import type { OrgMember } from '@/lib/api/types';

// Loads the org's members (all pages) once and caches them under ['org-members',
// orgId]. Shared by assignee pickers, @mention autocomplete and any place that
// renders a user id as a name. Project-member scoping is not applied — the org
// member set is the superset the sitemap's "project members" maps onto here
// (ADR-018); project-only filtering can be layered later via /projects/{id}/members.

async function fetchAllMembers(orgId: string): Promise<OrgMember[]> {
  const all: OrgMember[] = [];
  let cursor = '';
  // Bounded loop so a huge org can't spin forever; a few pages cover any UI need.
  for (let i = 0; i < 20; i++) {
    const page = await listOrgMembers(orgId, { cursor, limit: 100 });
    all.push(...page.items);
    if (!page.next_cursor) break;
    cursor = page.next_cursor;
  }
  return all;
}

export interface OrgMembers {
  members: OrgMember[];
  byId: Map<string, OrgMember>;
  isLoading: boolean;
}

export function useOrgMembers(): OrgMembers {
  const { orgId } = useOrg();
  const { data, isLoading } = useQuery({
    queryKey: ['org-members', orgId],
    queryFn: () => fetchAllMembers(orgId),
    staleTime: 5 * 60 * 1000,
  });
  const members = data ?? [];
  const byId = new Map(members.map((m) => [m.user_id, m]));
  return { members, byId, isLoading };
}

/** Best-effort display name for a user id (falls back to email, then a short id). */
export function memberName(byId: Map<string, OrgMember>, userId: string | null | undefined): string {
  if (!userId) return 'Unassigned';
  const m = byId.get(userId);
  if (!m) return userId.slice(0, 8);
  return m.name || m.email || userId.slice(0, 8);
}
