import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Privacy Policy' };

export default function PrivacyPage() {
  return (
    <LegalPage title="Privacy Policy" updated="September 17, 2026">
      <p>
        This policy explains what personal data Fluxboard collects, why it is processed, how
        long it is kept, and which rights you have over it. It applies to visitors of our
        website and to registered users of the service. Processing is carried out in accordance
        with Law No. 6698 on the Protection of Personal Data and, for users in the European
        Economic Area, the General Data Protection Regulation. Where the two frameworks differ,
        the stricter applicable standard is followed.
      </p>
      <h2>1. Data controller</h2>
      <p>
        The data controller for account and service data is the Fluxboard operating entity
        identified in your order confirmation. Customers that invite team members or otherwise
        input personal data of third parties into the service act as separate data controllers
        for that data; Fluxboard processes it as a data processor under the Data Processing
        Addendum. For any privacy question, including requests under Article 11 of Law No.
        6698, contact privacy@fluxboard.local. Requests are answered free of charge within
        thirty days at the latest.
      </p>
      <h2>2. Data we collect</h2>
      <p>
        We collect only what is needed to run the service. Account data includes your name,
        email address, password verifier, two-factor settings, and preferences. Content data
        includes the projects, tasks, comments, attachments, and organization settings you
        create. Operational data includes session records, audit events, authentication
        attempts, and billing metadata such as plan, metered usage counters, and invoice
        references. Payment card details are never collected or stored by us; they are handled
        directly by Stripe through hosted Checkout and the Billing Portal.
      </p>
      <h2>3. Purposes and legal bases</h2>
      <p>
        Account data is processed to perform the contract: creating and securing your account,
        providing access, and sending transactional messages such as verification links and
        security alerts. Content data is processed on the documented instructions of the
        customer organization. Operational and security data, including audit logs and
        authentication records, is processed on the basis of our legitimate interest in keeping
        the service secure and demonstrating compliance. Marketing communication, where offered,
        is sent only with your explicit consent, which you may withdraw at any time with effect
        for the future.
      </p>
      <h2>4. Cookies and similar technologies</h2>
      <p>
        We use a strictly necessary session cookie carrying the refresh token, which is marked
        httpOnly and is required for sign-in to work. A local theme preference is stored on
        your device and never leaves the browser. We do not use third-party advertising or
        cross-site tracking cookies. Any future introduction of analytics or marketing cookies
        will be announced in advance and activated only after your explicit consent, with an
        equally simple way to withdraw it.
      </p>
      <h2>5. Sharing and sub-processors</h2>
      <p>
        Personal data is not sold and is not shared for third-party marketing. It is disclosed
        only to sub-processors that are necessary to operate the service: cloud infrastructure
        for hosting and storage, Stripe for payment processing, and the email delivery provider
        used for transactional messages. Each sub-processor is bound by a written agreement
        limiting processing to documented instructions. A current list of sub-processors,
        including their function and hosting region, is maintained in the Data Processing
        Addendum and updated before any change takes effect.
      </p>
      <h2>6. International transfers</h2>
      <p>
        Service data is hosted within Türkiye by default. Where a sub-processor transfer
        outside Türkiye is unavoidable, for example for payment processing, the transfer takes
        place only with a lawful mechanism: an adequacy decision where available, otherwise
        standard contractual clauses supplemented by technical measures such as encryption in
        transit and at rest, or your explicit consent obtained in advance with clear
        information about the destination country and the associated risks, in line with
        Article 9 of Law No. 6698.
      </p>
      <h2>7. Retention and deletion</h2>
      <p>
        Active account and content data is kept for the duration of the subscription. Deleted
        tasks and attachments remain recoverable from the trash for thirty days and are then
        purged automatically. Closed accounts and deleted organizations enter a short
        wind-down window for operational recovery, after which personal data is deleted or
        anonymized. Billing records, invoices, and audit events that the law requires us to
        keep, for example under tax legislation, are retained for the statutory period, up to
        ten years, with access restricted to authorized personnel only.
      </p>
      <h2>8. Security measures</h2>
      <p>
        Access is isolated per tenant at the database level through row-level security, with
        additional application-layer checks. Passwords are stored as Argon2id hashes, never in
        plain text. Connections are encrypted in transit, secrets are encrypted at rest, and
        sessions can be revoked individually from the account page. Administrative access is
        limited, logged, and reviewed. Suspected vulnerabilities can be reported to
        privacy@fluxboard.local and are triaged promptly.
      </p>
      <h2>9. Your rights</h2>
      <p>
        Under Article 11 of Law No. 6698 and, where applicable, the GDPR, you have the right to
        learn whether your data is processed, to request information about it, to learn the
        purpose of processing and whether it is used accordingly, to know the third parties to
        whom it is transferred domestically or abroad, to request correction of incomplete or
        inaccurate data, to request deletion or destruction under the statutory conditions, to
        object to automated analysis that produces an adverse result, and to claim compensation
        for unlawful processing. You can update your profile, rotate credentials, review
        sessions, and export the audit log of your organization directly in the application at
        any time. Formal requests can be sent to privacy@fluxboard.local from your registered
        email address; identity may be verified before disclosure. You also have the right to
        lodge a complaint with the Personal Data Protection Authority.
      </p>
      <h2>10. Children</h2>
      <p>
        The service is intended for business use and is not directed at children. We do not
        knowingly collect data from individuals under the age of eighteen. If such data is
        discovered, it is deleted promptly after verification.
      </p>
      <h2>11. Changes to this policy</h2>
      <p>
        Material changes to this policy will be announced at least thirty days in advance
        through the application or by email. The version history is preserved and the previous
        wording remains available on request. Continued use of the service after the effective
        date constitutes acknowledgment of the updated policy.
      </p>
    </LegalPage>
  );
}
