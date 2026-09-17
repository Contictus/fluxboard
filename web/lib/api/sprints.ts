import { apiFetch } from './client';

/** One sprint (GET .../projects/{id}/sprints -> { sprints: [...] }). */
export interface Sprint {
  id: string;
  name: string;
  goal: string;
  status: 'planned' | 'active' | 'completed';
  started_at?: string | null;
  ended_at?: string | null;
  completed_total: number;
  completed_done: number;
  created_at: string;
}

/** Payload for POST .../projects/{id}/sprints. */
export interface CreateSprintInput {
  name: string;
  goal?: string;
  started_at?: string | null;
  ended_at?: string | null;
}

/** List sprints oldest-first. */
export async function listSprints(orgId: string, projectId: string): Promise<Sprint[]> {
  const res = await apiFetch<{ sprints: Sprint[] }>(`/orgs/${orgId}/projects/${projectId}/sprints`);
  return res.sprints ?? [];
}

/** Create a planned sprint (201, LEAD). */
export function createSprint(orgId: string, projectId: string, input: CreateSprintInput): Promise<Sprint> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/sprints`, { method: 'POST', body: input });
}

/** Start a planned sprint (LEAD). 409 when another sprint is active. */
export function startSprint(orgId: string, projectId: string, sprintId: string): Promise<Sprint> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/sprints/${sprintId}/start`, { method: 'POST' });
}

/** Complete the active sprint (LEAD). Snapshots velocity, backlogs the rest. */
export function completeSprint(orgId: string, projectId: string, sprintId: string): Promise<Sprint> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/sprints/${sprintId}/complete`, {
    method: 'POST',
  });
}

/** Delete a planned sprint (204, LEAD). */
export function deleteSprint(orgId: string, projectId: string, sprintId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/sprints/${sprintId}`, { method: 'DELETE' });
}

/** Assign tasks to a sprint (null = backlog) (204, LEAD). */
export function assignSprint(
  orgId: string,
  projectId: string,
  input: { task_ids: string[]; sprint_id: string | null },
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/sprints/assign`, {
    method: 'POST',
    body: input,
  });
}
