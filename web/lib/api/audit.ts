import { apiFetch } from './client';
import type { AuditEntry } from './types';

// Org audit-log viewer + CSV export (docs/08 §6, FR-AUD-003). Both ADMIN-only
// (read:audit); the usecase forces the org_id filter server-side.

export interface AuditFilter {
  actor?: string;
  action?: string;
  severity?: string;
  /** RFC3339 lower bound. */
  since?: string;
  /** RFC3339 upper bound. */
  until?: string;
  limit?: number;
}

export function toQuery(filter: AuditFilter): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(filter)) {
    if (v !== undefined && v !== '' && v !== null) qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : '';
}

export async function listAudit(orgId: string, filter: AuditFilter = {}): Promise<AuditEntry[]> {
  const res = await apiFetch<{ entries: AuditEntry[] }>(`/orgs/${orgId}/audit${toQuery(filter)}`);
  return res.entries;
}

/**
 * Fetch the CSV export as text. apiFetch returns the raw body for non-JSON
 * responses; the caller wraps it in a Blob to trigger a download (a plain
 * `<a download>` can't attach the in-memory bearer token).
 */
export function auditCsv(orgId: string, filter: AuditFilter = {}): Promise<string> {
  return apiFetch<string>(`/orgs/${orgId}/audit.csv${toQuery(filter)}`);
}
