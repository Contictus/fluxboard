import { apiFetch } from './client';
import type { Label } from './types';

// Org labels (docs/08 §5, FR-TASK-004). Full CRUD lands in §7 org settings; the
// board only needs to list them for the filter + bulk-label action.

export async function listLabels(orgId: string): Promise<Label[]> {
  const res = await apiFetch<{ labels: Label[] }>(`/orgs/${orgId}/labels`);
  return res.labels;
}
