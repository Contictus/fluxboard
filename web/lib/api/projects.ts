import { apiFetch } from './client';
import type { Project } from './types';

// Projects surface (docs/08 §5). The org shell (§4) only lists; create/board/
// settings land in Phase 7 §5.

export async function listProjects(
  orgId: string,
  opts: { archived?: boolean } = {},
): Promise<Project[]> {
  const qs = opts.archived ? '?archived=true' : '';
  const res = await apiFetch<{ projects: Project[] }>(`/orgs/${orgId}/projects${qs}`);
  return res.projects;
}
