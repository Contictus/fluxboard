import { apiFetch } from './client';

// Governed AI surface (ADR-025, FR-AI-001..008). All payloads are snake_case
// like the rest of the API. planDraft mints its own Idempotency-Key so a
// double-click never drafts twice.

export interface AIParseResult {
  run_id: string;
  model: string;
  tokens: number;
  items: string[];
}

export interface AIPlanResult {
  run_id: string;
  replayed: boolean;
  model: string;
  text: string;
  items: string[];
}

export interface AIDigestStats {
  project_id: string;
  total: number;
  overdue: number;
  unassigned: number;
  urgent_high: number;
  by_priority: Record<string, number>;
}

export interface AIDigestResult {
  run_id: string;
  text: string;
  stats: AIDigestStats;
}

export interface AIChatResult {
  run_id: string;
  model: string;
  text: string;
  tokens: number;
}

export interface AIRiskSignal {
  code: string;
  detail: string;
}

export interface AIRisk {
  id: string;
  project_id: string;
  task_id: string;
  score: 'low' | 'medium' | 'high';
  signals: AIRiskSignal[];
  created_at: string;
  updated_at: string;
}

/** NL text → task drafts (FR-AI-001). */
export function parseTasks(orgId: string, input: string): Promise<AIParseResult> {
  return apiFetch(`/orgs/${orgId}/ai/parse`, { method: 'POST', body: { input } });
}

/** Brief → structured plan draft (FR-AI-002, idempotent). */
export function planDraft(orgId: string, brief: string, key?: string): Promise<AIPlanResult> {
  const idempotencyKey =
    key ?? (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}`);
  return apiFetch(`/orgs/${orgId}/ai/plan`, {
    method: 'POST',
    body: { brief },
    headers: { 'Idempotency-Key': idempotencyKey },
  });
}

/** One chat turn with bounded history. */
export function chatTurn(orgId: string, message: string, history: string[] = []): Promise<AIChatResult> {
  return apiFetch(`/orgs/${orgId}/ai/chat`, { method: 'POST', body: { message, history } });
}

/** Sponsor-ready digest from live project data (FR-AI-003). */
export function projectDigest(orgId: string, projectId: string): Promise<AIDigestResult> {
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/ai/digest`);
}

/** Recompute open risks for a project (FR-AI-004, BUSINESS+). */
export async function scanRisks(orgId: string, projectId: string): Promise<AIRisk[]> {
  const res = await apiFetch<{ risks: AIRisk[] }>(`/orgs/${orgId}/ai/risks/scan`, {
    method: 'POST',
    body: { project_id: projectId },
  });
  return res.risks ?? [];
}

/** Open risks for a project (risk board). */
export async function listRisks(orgId: string, projectId: string): Promise<AIRisk[]> {
  const res = await apiFetch<{ risks: AIRisk[] }>(
    `/orgs/${orgId}/ai/risks?project_id=${encodeURIComponent(projectId)}`,
  );
  return res.risks ?? [];
}

/** Dismiss an open risk (204). */
export function dismissRisk(orgId: string, taskId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/ai/risks/dismiss`, { method: 'POST', body: { task_id: taskId } });
}

export interface AIApplyItem {
  title: string;
  description?: string;
  assignee_id?: string | null;
  priority?: string;
  start_date?: string | null;
  due_date?: string | null;
}

export interface AIAppliedTask {
  id: string;
  number: number;
  title: string;
}

export interface AIApplyResult {
  run_id: string;
  replayed: boolean;
  tasks: AIAppliedTask[];
}

/** Materialize draft items as tasks (FR-AI-009, idempotent, ≤ 50 items). */
export function applyPlan(
  orgId: string,
  input: { project_id: string; column_id: string; items: AIApplyItem[] },
  key?: string,
): Promise<AIApplyResult> {
  const idempotencyKey =
    key ?? (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}`);
  return apiFetch(`/orgs/${orgId}/ai/plan/apply`, {
    method: 'POST',
    body: input,
    headers: { 'Idempotency-Key': idempotencyKey },
  });
}
