import { apiFetch } from './client';
import type { CreateProjectInput, Project, UpdateProjectInput } from './types';

// Projects surface (docs/08 §5). Projects are addressed by UUID server-side;
// there is NO by-key route, so the frontend's `/projects/{projectKey}` URLs
// resolve the key to a project by listing (ADR-017, `resolveProjectByKey`).

export async function listProjects(
  orgId: string,
  opts: { archived?: boolean } = {},
): Promise<Project[]> {
  const qs = opts.archived ? '?archived=true' : '';
  const res = await apiFetch<{ projects: Project[] }>(`/orgs/${orgId}/projects${qs}`);
  return res.projects;
}

export function createProject(orgId: string, input: CreateProjectInput): Promise<Project> {
  return apiFetch(`/orgs/${orgId}/projects`, { method: 'POST', body: input });
}

export function updateProject(
  orgId: string,
  projectId: string,
  input: UpdateProjectInput,
): Promise<Project> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}`, { method: 'PATCH', body: input });
}

export function archiveProject(orgId: string, projectId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/archive`, { method: 'POST' });
}

export function unarchiveProject(orgId: string, projectId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/unarchive`, { method: 'POST' });
}

/**
 * Resolve a `/projects/{projectKey}` URL segment to a project. Keys are stored
 * upper-cased server-side; the caller passes the segment verbatim and we match
 * case-insensitively across active + archived. Returns null when absent.
 */
export async function resolveProjectByKey(orgId: string, key: string): Promise<Project | null> {
  const [active, archived] = await Promise.all([
    listProjects(orgId, {}),
    listProjects(orgId, { archived: true }),
  ]);
  const wanted = key.toUpperCase();
  // The archived list includes active projects too on this API, but dedupe defensively.
  const all = new Map<string, Project>();
  for (const p of [...active, ...archived]) all.set(p.id, p);
  return [...all.values()].find((p) => p.key.toUpperCase() === wanted) ?? null;
}
