import { apiFetch } from './client';
import type { Board, BulkActionInput, Column, CreateTaskInput, Task } from './types';

// Board / column / task-move surface (docs/08 §5, FR-PROJ-004/005). The board
// projection is fetched whole and cached under ['board', projectId]; drag-drop
// moves are optimistic and the rank key is minted client-side (lib/board/rank).

export async function getBoard(orgId: string, projectId: string): Promise<Board> {
  const board = await apiFetch<Board>(`/orgs/${orgId}/projects/${projectId}/board`);
  // The API may encode empty lists as null (Go nil slices); normalize so
  // board consumers can assume arrays (new projects have empty columns).
  return {
    ...board,
    columns: (board.columns ?? []).map((col) => ({
      ...col,
      tasks: (col.tasks ?? []).map((t) => ({
        ...t,
        labels: t.labels ?? [],
        comment_count: t.comment_count ?? 0,
        subtask_total: t.subtask_total ?? 0,
        subtask_done: t.subtask_done ?? 0,
      })),
    })),
  };
}

export function createColumn(
  orgId: string,
  projectId: string,
  input: { name: string; wip_limit?: number | null },
): Promise<Column> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/columns`, {
    method: 'POST',
    body: input,
  });
}

export function renameColumn(
  orgId: string,
  projectId: string,
  columnId: string,
  input: { name: string; wip_limit?: number | null },
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/columns/${columnId}`, {
    method: 'PATCH',
    body: input,
  });
}

export function reorderColumn(
  orgId: string,
  projectId: string,
  columnId: string,
  rank: string,
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/columns/${columnId}/position`, {
    method: 'PATCH',
    body: { rank },
  });
}

export function deleteColumn(
  orgId: string,
  projectId: string,
  columnId: string,
  targetColumnId: string,
): Promise<void> {
  const qs = targetColumnId ? `?target=${encodeURIComponent(targetColumnId)}` : '';
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/columns/${columnId}${qs}`, {
    method: 'DELETE',
  });
}

export function createTask(
  orgId: string,
  projectId: string,
  input: CreateTaskInput,
): Promise<Task> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/tasks`, {
    method: 'POST',
    body: input,
  });
}

/** Move a task on the board. 204 on success; 409 on a rank collision. */
export function moveTask(
  orgId: string,
  taskId: string,
  input: { column_id: string; rank: string },
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/position`, {
    method: 'PATCH',
    body: input,
  });
}

export function bulkTasks(orgId: string, input: BulkActionInput): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/bulk`, { method: 'POST', body: input });
}
