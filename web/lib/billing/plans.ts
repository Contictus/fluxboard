import type { PlanCode } from '@/lib/api/types';

// Client-side plan reference (docs/06 §2). There is NO public plan-catalog /
// pricing endpoint — the entitlement numbers here mirror the domain plan table
// (backend/internal/domain/billing) for display only; the *authoritative* price a
// switch will cost comes from POST /billing/preview-change (proration), and the
// checkout amount from Stripe. Keep these limits in sync with the Go plan table.
// -1 means unlimited. See ADR-020.

export interface PlanRef {
  code: PlanCode;
  name: string;
  tagline: string;
  maxMembers: number;
  maxProjects: number;
  maxStorageBytes: number;
  apiRatePerMin: number;
  auditRetentionDays: number;
  metered: boolean;
  /** Ordinal for upgrade/downgrade comparison. */
  tier: number;
}

export const PLANS: PlanRef[] = [
  {
    code: 'free',
    name: 'Free',
    tagline: 'For trying things out and small teams.',
    maxMembers: 5,
    maxProjects: 3,
    maxStorageBytes: 2 * 2 ** 30,
    apiRatePerMin: 60,
    auditRetentionDays: 7,
    metered: false,
    tier: 0,
  },
  {
    code: 'pro',
    name: 'Pro',
    tagline: 'For growing teams that need more room.',
    maxMembers: 25,
    maxProjects: 50,
    maxStorageBytes: 50 * 2 ** 30,
    apiRatePerMin: 300,
    auditRetentionDays: 30,
    metered: false,
    tier: 1,
  },
  {
    code: 'business',
    name: 'Business',
    tagline: 'Unlimited scale with usage-based metering.',
    maxMembers: -1,
    maxProjects: -1,
    maxStorageBytes: 500 * 2 ** 30,
    apiRatePerMin: 1200,
    auditRetentionDays: 365,
    metered: true,
    tier: 2,
  },
];

export function planByCode(code: string): PlanRef | undefined {
  return PLANS.find((p) => p.code === code);
}

/** Human count, or "Unlimited" for the -1 sentinel. */
export function limitLabel(n: number): string {
  return n < 0 ? 'Unlimited' : n.toLocaleString();
}
