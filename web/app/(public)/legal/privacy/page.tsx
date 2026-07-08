import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Privacy Policy' };

export default function PrivacyPage() {
  return (
    <LegalPage title="Privacy Policy" updated="July 8, 2026">
      <p>
        This policy describes how Fluxboard handles your data. This is a portfolio demonstration
        project; the text below is placeholder content.
      </p>
      <h2>Data we collect</h2>
      <p>
        Account details (name, email), the content you create (projects, tasks, comments), and
        operational metadata such as sessions and audit events used to secure your account.
      </p>
      <h2>How we use it</h2>
      <p>
        To provide the service, enforce access control, meter billing usage, and send
        transactional email such as verification and notifications.
      </p>
      <h2>Payment data</h2>
      <p>
        Card details never touch our servers. All payments are handled by Stripe via hosted
        Checkout and the Billing Portal.
      </p>
      <h2>Your rights</h2>
      <p>
        You can access and update your profile, export your organization&apos;s audit log, and
        delete your account at any time.
      </p>
    </LegalPage>
  );
}
