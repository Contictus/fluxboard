import { apiFetch } from './client';
import type {
  AdminTenant,
  AdminTenantDetail,
  AdminTenantFilter,
  AuditEntry,
} from './types';
import type { AuditFilter } from './audit';
import { toQuery } from './audit';

// Platform-admin surface (docs/build/PHASE-6 §5, FR-ADM-002..006). These routes
// live on the SEPARATE /admin router — NOT under a tenant/org — gated by the
// platform-admin guard (platform_role=admin + 2FA). A non-admin caller gets 401.

export async function listTenants(filter: AdminTenantFilter = {}): Promise<AdminTenant[]> {
  const qs = new URLSearchParams();
  if (filter.search) qs.set('search', filter.search);
  if (filter.plan) qs.set('plan', filter.plan);
  if (filter.status) qs.set('status', filter.status);
  if (filter.limit) qs.set('limit', String(filter.limit));
  if (filter.offset) qs.set('offset', String(filter.offset));
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  const res = await apiFetch<{ tenants: AdminTenant[] }>(`/admin/tenants${suffix}`);
  return res.tenants;
}

export function getTenantDetail(orgId: string): Promise<AdminTenantDetail> {
  return apiFetch(`/admin/tenants/${orgId}`);
}

/** Mint a read-only impersonation token for the target org. */
export async function startImpersonation(
  orgId: string,
): Promise<{ token: string; expires_at: string }> {
  return apiFetch(`/admin/tenants/${orgId}/impersonate`, { method: 'POST' });
}

/** Re-enqueue a Stripe webhook for reprocessing (202). */
export function retryWebhook(orgId: string, eventId: string): Promise<{ enqueued: string }> {
  return apiFetch(`/admin/tenants/${orgId}/webhooks/${eventId}/retry`, { method: 'POST' });
}

export function setFlag(orgId: string, flag: string, enabled: boolean): Promise<void> {
  return apiFetch(`/admin/tenants/${orgId}/flags`, { method: 'PUT', body: { flag, enabled } });
}

export function setOverride(
  orgId: string,
  input: { key: string; value: string; note: string },
): Promise<void> {
  return apiFetch(`/admin/tenants/${orgId}/overrides`, { method: 'PUT', body: input });
}

export function deleteOverride(orgId: string, key: string): Promise<void> {
  return apiFetch(`/admin/tenants/${orgId}/overrides/${encodeURIComponent(key)}`, {
    method: 'DELETE',
  });
}

/** Cross-tenant audit log (global). */
export async function globalAudit(filter: AuditFilter = {}): Promise<AuditEntry[]> {
  const res = await apiFetch<{ entries: AuditEntry[] }>(`/admin/audit${toQuery(filter)}`);
  return res.entries;
}

/** Asynq queue summary. Shape varies; `available:false` when the inspector is off. */
export function getJobs(): Promise<Record<string, unknown>> {
  return apiFetch(`/admin/jobs`);
}
