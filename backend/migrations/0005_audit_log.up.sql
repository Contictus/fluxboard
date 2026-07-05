-- 0005_audit_log — append-only audit trail (docs/07 §4, FR-AUD-001/002).
--
-- Written by the app on security-relevant events (auth reuse, password/email
-- change, session revoke, membership & role changes, invitations, org lifecycle).
-- The org-scoped viewer is Phase 6; this migration only lands the write target.
--
-- NOT a [T] tenant table: org_id is nullable (platform-level events have none) and
-- rows are written before/without a tenant GUC. Isolation for the future viewer is
-- enforced by an explicit org_id filter in the read query, not RLS. Append-only is
-- enforced at the grant level: the app role may INSERT/SELECT but never UPDATE or
-- DELETE. Retention purge (FR-AUD-002) runs later as a separate privileged job.

CREATE TABLE audit_log (
  id                   uuid PRIMARY KEY,
  org_id               uuid,                        -- NULL for platform-level events
  actor_user_id        uuid,
  impersonator_user_id uuid,
  action               text NOT NULL,               -- 'auth.refresh_reuse', 'member.role_change', ...
  target_type          text,
  target_id            text,
  metadata             jsonb NOT NULL DEFAULT '{}',
  ip                   inet,
  user_agent           text,
  severity             text NOT NULL DEFAULT 'info'
                       CHECK (severity IN ('info','warning','security')),
  created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_org_idx ON audit_log (org_id, created_at DESC);
CREATE INDEX audit_log_security_idx ON audit_log (severity, created_at DESC) WHERE severity = 'security';

REVOKE UPDATE, DELETE ON audit_log FROM fluxboard_app;   -- append-only for app role
