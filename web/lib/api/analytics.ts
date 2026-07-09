import { apiFetch } from './client';
import type { ProjectAnalytics, UsageDashboard } from './types';

// Analytics surface (FR-AN-001/002). Both read nightly rollups, never live
// aggregation. Project analytics = any member (read:org); org usage = ADMIN+
// (read:billing), so the usage caller must guard on role.

/** Org usage dashboard (seats/storage/api-calls + estimate). ADMIN+ only. */
export function getUsageDashboard(orgId: string): Promise<UsageDashboard> {
  return apiFetch(`/orgs/${orgId}/usage`);
}

/** A project's rollup analytics over an optional [from,to] day window (YYYY-MM-DD). */
export function getProjectAnalytics(
  orgId: string,
  projectId: string,
  window: { from?: string; to?: string } = {},
): Promise<ProjectAnalytics> {
  const qs = new URLSearchParams();
  if (window.from) qs.set('from', window.from);
  if (window.to) qs.set('to', window.to);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return apiFetch(`/orgs/${orgId}/projects/${projectId}/analytics${suffix}`);
}
