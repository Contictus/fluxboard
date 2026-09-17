import { apiFetch } from './client';

/** One side of a dependency edge. */
export interface TaskLink {
  task_id: string;
  number: number;
  title: string;
  column_id: string;
}

/** Both directions (GET .../tasks/{id}/links). */
export interface TaskLinks {
  blockers: TaskLink[];
  blocked: TaskLink[];
}

/** List both directions. */
export async function listTaskLinks(orgId: string, taskId: string): Promise<TaskLinks> {
  const res = await apiFetch<TaskLinks>(`/orgs/${orgId}/tasks/${taskId}/links`);
  return { blockers: res.blockers ?? [], blocked: res.blocked ?? [] };
}

/** Record taskID blocked-by linkedID (204, CONTRIBUTOR). */
export function addTaskLink(orgId: string, taskId: string, linkedTaskId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/links`, {
    method: 'POST',
    body: { linked_task_id: linkedTaskId },
  });
}

/** Remove an edge (204, CONTRIBUTOR). */
export function removeTaskLink(orgId: string, taskId: string, linkedTaskId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/links/${linkedTaskId}`, { method: 'DELETE' });
}
