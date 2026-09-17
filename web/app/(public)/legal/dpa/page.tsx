import type { Metadata } from 'next';

import { LegalPage } from '@/components/marketing/legal-page';

export const metadata: Metadata = { title: 'Data Processing Addendum' };

export default function DpaPage() {
  return (
    <LegalPage title="Data Processing Addendum" updated="September 17, 2026">
      <p>
        This Data Processing Addendum forms part of the agreement between the customer and
        Fluxboard and governs the processing of personal data that the customer entrusts to the
        service. It is designed to satisfy Article 28 of the GDPR and the corresponding
        obligations under Law No. 6698 on the Protection of Personal Data. In case of conflict
        between this addendum and the main agreement regarding data protection, this addendum
        prevails. Defined terms carry the meaning given in the applicable data-protection law.
      </p>
      <h2>1. Subject matter and duration</h2>
      <p>
        The processing covers account identities, team memberships, customer content such as
        projects, tasks, comments, and attachments, and related operational metadata that the
        customer uploads or generates through the service. Processing lasts for the term of the
        subscription plus the wind-down and retention periods described in Section 8. Processing
        operations include hosting, storage, backup, retrieval, transmission to sub-processors
        listed in Section 5, and deletion. No other processing takes place.
      </p>
      <h2>2. Roles of the parties</h2>
      <p>
        The customer acts as the data controller, or the veri sorumlusu, and determines the
        purposes and means of processing customer content. Fluxboard acts as the data
        processor, or the veri işleyen, and processes personal data only on documented
        instructions from the customer. The service interface, API, and configuration settings
        constitute documented instructions. If an instruction appears to infringe applicable
        data-protection law, Fluxboard will inform the customer promptly and suspend the
        affected processing until clarified, to the extent legally permitted.
      </p>
      <h2>3. Confidentiality and personnel</h2>
      <p>
        Personal data is accessible only to personnel who need it to operate and support the
        service. All such personnel are bound by written confidentiality obligations that
        survive the end of their engagement. Administrative access to production systems is
        granted individually, follows the principle of least privilege, and is recorded in
        tamper-evident audit logs that are reviewed regularly.
      </p>
      <h2>4. Security measures</h2>
      <p>
        Fluxboard implements appropriate technical and organizational measures, including
        tenant isolation enforced by database row-level security with defense-in-depth
        application checks, encryption in transit for all connections, encryption of secrets
        at rest, Argon2id password hashing, optional two-factor authentication, short-lived
        access tokens with rotating httpOnly refresh cookies, individual session revocation,
        network segmentation between application, data, and worker tiers, regular dependency
        and vulnerability scanning, and immutable audit logging of security-relevant events.
        Measures are reviewed at least annually and after every significant incident.
      </p>
      <h2>5. Sub-processors</h2>
      <p>
        The customer authorizes the following categories of sub-processors: cloud
        infrastructure for compute, database, cache, and object storage hosted in Türkiye;
        Stripe for payment processing, which receives only the data required to complete
        checkout and manage subscriptions; and the email delivery provider used for
        transactional messages such as verification links and notifications. The current
        sub-processor list with provider names, functions, and hosting regions is published in
        the trust center and updated at least thirty days before any addition or replacement,
        during which the customer may object on reasonable data-protection grounds.
      </p>
      <h2>6. Personal data breach</h2>
      <p>
        Fluxboard will notify the customer without undue delay, and no later than seventy-two
        hours after becoming aware of a personal data breach affecting customer data. The
        notification describes the nature of the breach, the categories and approximate number
        of data subjects and records concerned, the likely consequences, and the measures taken
        or proposed to mitigate harm. Where the breach is likely to result in high risk to
        individuals, Fluxboard assists the customer in meeting notification duties toward the
        Personal Data Protection Authority and affected data subjects. Forensic findings and
        remediation reports are shared as they become available.
      </p>
      <h2>7. Assistance with data-subject rights</h2>
      <p>
        Taking into account the nature of the processing, Fluxboard assists the customer in
        fulfilling requests under Article 11 of Law No. 6698 and Chapter III of the GDPR,
        including access, rectification, erasure, restriction, portability, and objection, by
        providing self-service export, correction, and deletion capabilities in the application
        and, where self-service is insufficient, by executing verified customer instructions
        within the statutory time limits. Requests received directly from data subjects are
        forwarded to the customer without undue delay.
      </p>
      <h2>8. Return and deletion</h2>
      <p>
        During the subscription, the customer can export organization data, including the audit
        log, at any time. Soft-deleted content stays recoverable in the trash for thirty days
        and is then purged automatically, including nightly cleanup of orphaned attachments.
        After termination, data enters a short operational wind-down window and is then deleted
        or irreversibly anonymized across production and backup systems within ninety days,
        except for billing records, invoices, and audit events that must be retained under tax
        or other statutory duties, for up to ten years with strictly limited access. Written
        confirmation of deletion is provided on request.
      </p>
      <h2>9. Audit rights</h2>
      <p>
        Once per contract year, and additionally after a confirmed breach, the customer may
        audit compliance with this addendum, either directly or through an independent auditor
        bound by confidentiality, with at least thirty days notice and during business hours.
        Fluxboard contributes the documentation reasonably required, including records of
        processing activities, sub-processor agreements, and summaries of recent security
        assessments. Findings that reveal non-compliance are remediated under a mutually agreed
        plan.
      </p>
      <h2>10. Liability and term</h2>
      <p>
        Liability under this addendum follows the liability provisions of the main agreement,
        except that statutory data-protection liability that cannot be limited remains
        unaffected. This addendum enters into force with the main agreement and remains
        effective until all customer personal data has been returned or deleted in accordance
        with Section 8. Obligations that by nature survive, including confidentiality, audit
        cooperation for the retention period, and assistance with pending breach matters,
        remain in force after termination.
      </p>
    </LegalPage>
  );
}
