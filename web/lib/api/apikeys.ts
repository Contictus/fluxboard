import { apiFetch } from './client';
import type { ApiKey, ApiKeyScope, CreateApiKeyResult } from './types';

// Org API keys (docs/08 §6, FR-API-001). All ADMIN-only (write:apikeys). The
// plaintext secret is returned exactly once, by createApiKey; thereafter only the
// public prefix is ever visible.

export async function listApiKeys(orgId: string): Promise<ApiKey[]> {
  const res = await apiFetch<{ api_keys: ApiKey[] }>(`/orgs/${orgId}/api-keys`);
  return res.api_keys;
}

/** Mint a key. The returned `secret` is shown once and never retrievable again. */
export function createApiKey(
  orgId: string,
  input: { name: string; scopes: ApiKeyScope[] },
): Promise<CreateApiKeyResult> {
  return apiFetch(`/orgs/${orgId}/api-keys`, { method: 'POST', body: input });
}

/** Revoke a key (204). */
export function revokeApiKey(orgId: string, id: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/api-keys/${id}`, { method: 'DELETE' });
}
