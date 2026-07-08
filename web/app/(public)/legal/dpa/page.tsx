import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Data Processing Addendum' };

export default function DpaPage() {
  return (
    <LegalPage title="Data Processing Addendum" updated="July 8, 2026">
      <p>
        This addendum describes the roles and safeguards for processing personal data on behalf of
        customers. This is a portfolio demonstration project; the text below is placeholder content.
      </p>
      <h2>Roles</h2>
      <p>
        The customer is the data controller; Fluxboard acts as the data processor for content
        stored in the customer&apos;s organization.
      </p>
      <h2>Sub-processors</h2>
      <p>
        We use Stripe for payment processing and standard cloud infrastructure providers for
        hosting, storage, and email delivery.
      </p>
      <h2>Security measures</h2>
      <p>
        Row-level tenant isolation, encryption in transit, hashed credentials, two-factor
        authentication, and audit logging.
      </p>
      <h2>Data deletion</h2>
      <p>
        Upon account or organization deletion, associated data is removed subject to retention
        rules for billing-relevant records.
      </p>
    </LegalPage>
  );
}
