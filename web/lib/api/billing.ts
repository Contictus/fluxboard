import { apiFetch } from './client';
import type {
  BillingSummary,
  ChangePreview,
  CheckoutInput,
  Invoice,
  PlanCode,
} from './types';

// Billing surface (docs/06, docs/08 §4). Every route is gated `read:billing` /
// `write:billing` = ADMIN+; a MEMBER/GUEST caller gets 403, so callers must guard
// on role before invoking. Money is integer minor units throughout.

/** Billing page snapshot (plan, status, period, entitlements). */
export function getBillingSummary(orgId: string): Promise<BillingSummary> {
  return apiFetch(`/orgs/${orgId}/billing/summary`);
}

/** Invoice mirror, newest-first (FR-BILL-008). */
export async function listInvoices(orgId: string): Promise<Invoice[]> {
  const res = await apiFetch<{ invoices: Invoice[] }>(`/orgs/${orgId}/billing/invoices`);
  return res.invoices;
}

/** Start hosted Stripe Checkout for a paid plan; returns the redirect URL. */
export async function startCheckout(orgId: string, input: CheckoutInput): Promise<string> {
  const res = await apiFetch<{ url: string }>(`/orgs/${orgId}/billing/checkout`, {
    method: 'POST',
    body: input,
  });
  return res.url;
}

/** Prorated amount for switching an existing subscription to `plan`. */
export function previewChange(orgId: string, plan: PlanCode): Promise<ChangePreview> {
  return apiFetch(`/orgs/${orgId}/billing/preview-change`, { method: 'POST', body: { plan } });
}

/** Apply a plan change on the existing subscription (204). */
export function applyChange(orgId: string, plan: PlanCode): Promise<void> {
  return apiFetch(`/orgs/${orgId}/billing/change`, { method: 'POST', body: { plan } });
}

/** Cancel the subscription — at period end (default) or immediately (204). */
export function cancelSubscription(orgId: string, atPeriodEnd = true): Promise<void> {
  return apiFetch(`/orgs/${orgId}/billing/cancel`, {
    method: 'POST',
    body: { at_period_end: atPeriodEnd },
  });
}

/** Clear a scheduled at-period-end cancellation (204). */
export function resumeSubscription(orgId: string): Promise<void> {
  return apiFetch(`/orgs/${orgId}/billing/resume`, { method: 'POST' });
}

/** Stripe Billing Portal URL for payment-method management; returns the URL. */
export async function billingPortal(orgId: string): Promise<string> {
  const res = await apiFetch<{ url: string }>(`/orgs/${orgId}/billing/portal`, { method: 'POST' });
  return res.url;
}
