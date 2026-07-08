// Static marketing plan data. Prices are integer minor units (cents) mirroring
// the stub seed (plans.monthly_price: free=0, pro=1200, business=4900). The live
// billing UI (Phase 8) reads real entitlements from the API.

export interface MarketingPlan {
  key: string;
  name: string;
  priceCents: number;
  tagline: string;
  features: string[];
  highlighted?: boolean;
}

export const plans: MarketingPlan[] = [
  {
    key: 'free',
    name: 'Free',
    priceCents: 0,
    tagline: 'For trying things out.',
    features: ['1 project', '3 members', '100 MB storage', 'Community support'],
  },
  {
    key: 'pro',
    name: 'Pro',
    priceCents: 1200,
    tagline: 'For growing teams.',
    features: ['20 projects', '15 members', '10 GB storage', 'Metered API access', 'Email support'],
    highlighted: true,
  },
  {
    key: 'business',
    name: 'Business',
    priceCents: 4900,
    tagline: 'For scale.',
    features: [
      'Unlimited projects',
      '100 members',
      '100 GB storage',
      'Higher rate limits',
      'Priority support',
      'Audit log export',
    ],
  },
];

/** Format integer cents as a whole-dollar price string (marketing display only). */
export function formatPrice(cents: number): string {
  if (cents === 0) return '$0';
  return `$${(cents / 100).toFixed(0)}`;
}
