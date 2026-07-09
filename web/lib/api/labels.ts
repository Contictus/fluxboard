import { apiFetch } from './client';
import type { Label } from './types';

// Org labels (docs/08 §5, FR-TASK-004). Reads under read:org; writes need
// write:labels (MEMBER+). CRUD surfaces in the §7 org-settings labels page.

export async function listLabels(orgId: string): Promise<Label[]> {
  const res = await apiFetch<{ labels: Label[] }>(`/orgs/${orgId}/labels`);
  return res.labels;
}

export interface LabelInput {
  name: string;
  color: string;
}

/** Create a label (201 labelResp, MEMBER+). */
export function createLabel(orgId: string, input: LabelInput): Promise<Label> {
  return apiFetch(`/orgs/${orgId}/labels`, { method: 'POST', body: input });
}

/** Rename / recolor a label (204, MEMBER+). */
export function updateLabel(orgId: string, id: string, input: LabelInput): Promise<void> {
  return apiFetch(`/orgs/${orgId}/labels/${id}`, { method: 'PATCH', body: input });
}

/** Delete a label (204, MEMBER+). */
export function deleteLabel(orgId: string, id: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/labels/${id}`, { method: 'DELETE' });
}
