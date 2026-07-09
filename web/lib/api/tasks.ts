import { apiFetch } from './client';
import type { Task } from './types';

// Task search (docs/08, FR-TASK-007). There is no server-side "assignee=me"
// alias, so the caller passes its own user id explicitly for the org-home
// "assigned to me" list (ADR-016). Full board/task surfaces land in §5/§6.

export interface TaskSearchParams {
  q?: string;
  project_id?: string;
  assignee_id?: string;
  column_id?: string;
  priority?: string;
  label_id?: string;
  limit?: number;
  offset?: number;
}

export interface TaskSearchResult {
  tasks: Task[];
  total: number;
}

export function searchTasks(
  orgId: string,
  params: TaskSearchParams = {},
): Promise<TaskSearchResult> {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') qs.set(k, String(v));
  }
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return apiFetch(`/orgs/${orgId}/tasks/search${suffix}`);
}
