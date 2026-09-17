import { apiFetch } from './client';
import type {
  Activity,
  Attachment,
  Comment,
  Label,
  Subtask,
  Task,
  UpdateTaskInput,
} from './types';

// Task surface (docs/08 §5, FR-TASK-001..009). Tasks are addressed by UUID under
// /orgs/{orgId}/tasks/{taskId}; the `/projects/{key}/tasks/{number}` URLs resolve
// number→id via the board projection (no by-number route — ADR-018). There is no
// server "assignee=me" alias, so callers pass their own user id (ADR-016).

// ---- Search (FR-TASK-007) -------------------------------------------------

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

// ---- Task detail (FR-TASK-001/002) ----------------------------------------

export function getTask(orgId: string, taskId: string): Promise<Task> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}`);
}

export function updateTask(orgId: string, taskId: string, input: UpdateTaskInput): Promise<Task> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}`, { method: 'PATCH', body: input });
}

export async function listActivity(orgId: string, taskId: string): Promise<Activity[]> {
  const res = await apiFetch<{ activity: Activity[] }>(`/orgs/${orgId}/tasks/${taskId}/activity`);
  return res.activity;
}

// ---- Trash (FR-TASK-009) --------------------------------------------------

export function trashTask(orgId: string, taskId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}`, { method: 'DELETE' });
}

export function restoreTask(orgId: string, taskId: string): Promise<Task> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/restore`, { method: 'POST' });
}

/** A project's trashed tasks. Org-wide trash is composed from these client-side. */
export async function listTrash(orgId: string, projectId: string): Promise<Task[]> {
  const res = await apiFetch<{ tasks: Task[] }>(`/orgs/${orgId}/projects/${projectId}/trash`);
  return res.tasks;
}

// ---- Subtasks (FR-TASK-003) -----------------------------------------------

export async function listSubtasks(orgId: string, taskId: string): Promise<Subtask[]> {
  const res = await apiFetch<{ subtasks: Subtask[] }>(`/orgs/${orgId}/tasks/${taskId}/subtasks`);
  return res.subtasks;
}

export function addSubtask(orgId: string, taskId: string, title: string): Promise<Subtask> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/subtasks`, {
    method: 'POST',
    body: { title, done: false },
  });
}

export function updateSubtask(
  orgId: string,
  taskId: string,
  subtaskId: string,
  input: { title: string; done: boolean },
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/subtasks/${subtaskId}`, {
    method: 'PATCH',
    body: input,
  });
}

export function deleteSubtask(orgId: string, taskId: string, subtaskId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/subtasks/${subtaskId}`, { method: 'DELETE' });
}

// ---- Comments (FR-TASK-005) -----------------------------------------------

export async function listComments(orgId: string, taskId: string): Promise<Comment[]> {
  const res = await apiFetch<{ comments: Comment[] }>(`/orgs/${orgId}/tasks/${taskId}/comments`);
  return res.comments;
}

export function addComment(orgId: string, taskId: string, body: string): Promise<Comment> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/comments`, { method: 'POST', body: { body } });
}

export function editComment(
  orgId: string,
  taskId: string,
  commentId: string,
  body: string,
): Promise<Comment> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/comments/${commentId}`, {
    method: 'PATCH',
    body: { body },
  });
}

export function deleteComment(orgId: string, taskId: string, commentId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/comments/${commentId}`, { method: 'DELETE' });
}

// ---- Time tracking (FR-TIME) --------------------------------------------------

/** One timer run or manual entry. */
export interface TimeEntry {
  id: string;
  task_id: string;
  user_id: string;
  started_at: string;
  ended_at?: string | null;
  note: string;
  seconds: number;
  created_at: string;
}

export async function listTimeEntries(orgId: string, taskId: string): Promise<TimeEntry[]> {
  const res = await apiFetch<{ entries: TimeEntry[] }>(`/orgs/${orgId}/tasks/${taskId}/time`);
  return res.entries ?? [];
}

/** Start a timer (stops the caller's other running timers). */
export function startTimer(orgId: string, taskId: string): Promise<TimeEntry> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/time/start`, { method: 'POST' });
}

/** Stop a running entry (owner only). */
export function stopTimer(orgId: string, taskId: string, entryId: string): Promise<TimeEntry> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/time/${entryId}/stop`, { method: 'POST' });
}

/** Log a finished manual entry. */
export function logTime(
  orgId: string,
  taskId: string,
  input: { started_at: string; ended_at: string; note?: string },
): Promise<TimeEntry> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/time`, { method: 'POST', body: input });
}

/** Delete an entry (owner only, 204). */
export function deleteTimeEntry(orgId: string, taskId: string, entryId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/time/${entryId}`, { method: 'DELETE' });
}

/** Sum seconds across entries (running timers counted to now). */
export function totalSeconds(entries: TimeEntry[]): number {
  const now = Date.now();
  return entries.reduce((sum, e) => {
    if (e.ended_at) return sum + e.seconds;
    return sum + Math.max(0, Math.floor((now - new Date(e.started_at).getTime()) / 1000));
  }, 0);
}

/** Format 3661 -> "1h 01m". */
export function formatDuration(totalSecs: number): string {
  const h = Math.floor(totalSecs / 3600);
  const m = Math.floor((totalSecs % 3600) / 60);
  if (h === 0) return `${m}m`;
  return `${h}h ${String(m).padStart(2, '0')}m`;
}

// ---- Task labels (FR-TASK-004) --------------------------------------------

export async function listTaskLabels(orgId: string, taskId: string): Promise<Label[]> {
  const res = await apiFetch<{ labels: Label[] }>(`/orgs/${orgId}/tasks/${taskId}/labels`);
  return res.labels;
}

export function attachLabel(orgId: string, taskId: string, labelId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/labels`, {
    method: 'POST',
    body: { label_id: labelId },
  });
}

export function detachLabel(orgId: string, taskId: string, labelId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/labels/${labelId}`, { method: 'DELETE' });
}

// ---- Attachments (FR-TASK-006) --------------------------------------------

export async function listAttachments(orgId: string, taskId: string): Promise<Attachment[]> {
  const res = await apiFetch<{ attachments: Attachment[] }>(
    `/orgs/${orgId}/tasks/${taskId}/attachments`,
  );
  return res.attachments;
}

export interface RequestUploadResult {
  attachment: Attachment;
  upload_url: string;
}

export function requestUpload(
  orgId: string,
  taskId: string,
  input: { filename: string; content_type: string; size: number },
): Promise<RequestUploadResult> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/attachments`, { method: 'POST', body: input });
}

export function confirmUpload(
  orgId: string,
  taskId: string,
  attachmentId: string,
): Promise<Attachment> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/attachments/${attachmentId}/confirm`, {
    method: 'POST',
  });
}

export async function attachmentDownloadUrl(
  orgId: string,
  taskId: string,
  attachmentId: string,
): Promise<string> {
  const res = await apiFetch<{ download_url: string }>(
    `/orgs/${orgId}/tasks/${taskId}/attachments/${attachmentId}/download`,
  );
  return res.download_url;
}

export function deleteAttachment(
  orgId: string,
  taskId: string,
  attachmentId: string,
): Promise<void> {
  return apiFetch(`/orgs/${orgId}/tasks/${taskId}/attachments/${attachmentId}`, {
    method: 'DELETE',
  });
}
