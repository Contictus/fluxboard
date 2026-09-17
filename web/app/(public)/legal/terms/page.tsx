import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Terms of Service' };

export default function TermsPage() {
  return (
    <LegalPage title="Terms of Service" updated="September 17, 2026">
      <p>
        These Terms of Service form a binding agreement between you and Fluxboard for the use
        of the hosted project-management service, including all web applications, APIs, and
        related billing functionality. By creating an account, accessing an organization, or
        otherwise using the service, you accept these terms in full. If you use the service on
        behalf of a company or other legal entity, you represent that you have the authority to
        bind that entity, and the words you and customer refer to that entity.
      </p>
      <h2>1. The service</h2>
      <p>
        Fluxboard provides multi-tenant workspaces for projects, boards, tasks, file
        attachments, and usage-based billing. We may modify, extend, or deprecate features over
        time. Material changes that reduce the functionality of a paid plan will be announced in
        advance through the application or by email, and customers may cancel in accordance with
        Section 6 if they do not accept the change. We aim for high availability but do not
        guarantee uninterrupted operation; the current operational status is published on the
        status page.
      </p>
      <h2>2. Accounts and organizations</h2>
      <p>
        You must provide a valid email address and keep your credentials confidential. Accounts
        are personal: sharing passwords or session tokens is prohibited. Enabling two-factor
        authentication is strongly recommended and may be required by your organization owner.
        Organizations are separate tenants. Joining or creating an organization does not grant
        access to any other organization, and roles assigned in one organization carry no rights
        in another. You are responsible for all activity performed under your account, including
        actions taken by integrations that use your API keys.
      </p>
      <h2>3. Acceptable use</h2>
      <p>
        You agree not to misuse the service. Prohibited conduct includes attempting to access
        data of other tenants, circumventing access controls or plan limits, reverse engineering
        non-public parts of the platform, sending unsolicited bulk email through the service,
        uploading unlawful or infringing content, or using the service in a way that degrades
        its reliability for others. Automated access beyond documented rate limits, credential
        stuffing, and security probing without prior written consent are prohibited. We may
        investigate suspected violations using audit and access logs.
      </p>
      <h2>4. Customer content and licenses</h2>
      <p>
        Content you create in the service, such as projects, tasks, comments, and attachments,
        remains yours. You grant Fluxboard a limited, worldwide license to host, replicate, and
        display that content solely to operate and secure the service, for the duration of your
        subscription plus a reasonable wind-down period. You represent that you hold the rights
        necessary to upload your content and that it does not violate applicable law. We do not
        claim ownership of customer content and do not use it to train machine-learning models.
      </p>
      <h2>5. Personal data</h2>
      <p>
        The processing of personal data is governed by the Privacy Policy and, where
        applicable, the Data Processing Addendum. Where you process personal data of third
        parties through the service, for example by inviting team members or uploading files
        that identify individuals, you act as the data controller for that processing and are
        responsible for having a lawful basis, including explicit consent where required under
        Law No. 6698 on the Protection of Personal Data. Fluxboard processes such data as a
        data processor on documented instructions.
      </p>
      <h2>6. Plans, billing, and taxes</h2>
      <p>
        Paid plans are billed through Stripe on a recurring basis. Usage-based charges are
        metered continuously and invoiced at the end of each billing cycle according to the
        rates published on the pricing page. Plan upgrades take effect immediately with pro
        rata charges; downgrades and cancellations take effect at the end of the current cycle
        unless stated otherwise. Failed payments may result in read-only restrictions or
        suspension after notice. Fees are exclusive of taxes, which are your responsibility.
        Card and bank details are processed exclusively by Stripe and never stored on
        Fluxboard servers.
      </p>
      <h2>7. Suspension and termination</h2>
      <p>
        You may close your account at any time from the account settings. If you are the sole
        owner of an organization, you must transfer ownership or delete the organization first.
        We may suspend or terminate access, with prior notice where feasible, for violations of
        these terms, prolonged non-payment, or legal obligations. Upon termination, your access
        ends immediately and customer content becomes subject to the deletion and retention
        rules described in the Data Processing Addendum, including longer retention of
        billing-relevant records where the law requires it.
      </p>
      <h2>8. Warranties and liability</h2>
      <p>
        The service is provided on an as-is and as-available basis to the maximum extent
        permitted by law. We disclaim implied warranties of merchantability, fitness for a
        particular purpose, and non-infringement. Nothing in these terms limits liability that
        cannot be limited under applicable law. To the extent permitted by law, our aggregate
        liability arising from the service is limited to the fees you paid in the twelve months
        preceding the claim, and we are not liable for indirect, incidental, or consequential
        damages such as loss of profit, revenue, or data.
      </p>
      <h2>9. Changes to these terms</h2>
      <p>
        We may update these terms to reflect legal, technical, or commercial changes. Material
        updates will be announced at least thirty days before they take effect, either in the
        application or by email to the account address. Continued use of the service after the
        effective date constitutes acceptance. If you do not agree with a material change, you
        may cancel and close your account before it takes effect.
      </p>
      <h2>10. Governing law and contact</h2>
      <p>
        These terms are governed by the laws of the Republic of Türkiye, without regard to
        conflict-of-law rules. Disputes that cannot be resolved amicably fall under the
        jurisdiction of the competent courts and enforcement offices. For questions about these
        terms, including data-protection matters, contact privacy@fluxboard.local. Notices sent
        to your registered email address are deemed received within a reasonable time after
        dispatch.
      </p>
    </LegalPage>
  );
}
