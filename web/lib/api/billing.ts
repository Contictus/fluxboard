import { apiFetch } from './client';
import type { BillingSummary } from './types';

// Billing surface (docs/06). Only the org-shell needs the summary here; the full
// billing UI (plans/checkout/usage) lands in Phase 7 §8.
//
// NOTE: GET /orgs/{id}/billing/summary is gated `read:billing` = ADMIN+; a
// MEMBER/GUEST caller gets 403. Callers must guard on role before invoking.

export function getBillingSummary(orgId: string): Promise<BillingSummary> {
  return apiFetch(`/orgs/${orgId}/billing/summary`);
}
