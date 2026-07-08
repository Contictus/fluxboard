import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Terms of Service' };

export default function TermsPage() {
  return (
    <LegalPage title="Terms of Service" updated="July 8, 2026">
      <p>
        These terms govern your use of Fluxboard. This is a portfolio demonstration project;
        the text below is placeholder content and does not constitute a binding agreement.
      </p>
      <h2>1. Accounts</h2>
      <p>
        You are responsible for safeguarding your account credentials and for all activity that
        occurs under your account. Enable two-factor authentication for additional protection.
      </p>
      <h2>2. Acceptable use</h2>
      <p>
        You agree not to misuse the service, attempt to disrupt it, or access data belonging to
        other tenants. Tenant isolation is enforced technically and contractually.
      </p>
      <h2>3. Billing</h2>
      <p>
        Paid plans are billed through Stripe. Usage-based charges are metered and reported each
        billing cycle. You may change or cancel your plan at any time.
      </p>
      <h2>4. Termination</h2>
      <p>
        You may delete your account at any time, subject to restrictions where you are the sole
        owner of an organization. We may suspend accounts that violate these terms.
      </p>
    </LegalPage>
  );
}
