-- 0004_tenancy_rls — Row-Level Security for the Phase 2 tenant-owned tables.
-- Kept separate from the DDL (docs/07 §5) so the isolation layer reviews as one
-- artifact. Pattern from docs/05 §4: predicate on the per-transaction GUC
-- app.current_tenant, which the TenantPool wrapper sets via SET LOCAL.
--
-- current_setting(..., true) returns NULL when the GUC is unset ⇒ predicate is
-- NULL ⇒ ZERO rows. Fail-closed: a request that forgot tenant context reads
-- nothing rather than everything.
--
-- ENABLE (not FORCE): the app role (fluxboard_app) is NOBYPASSRLS and never owns
-- these tables, so ENABLE fully isolates it. We deliberately do NOT FORCE, so
-- the owner retains a bypass — required by the SECURITY DEFINER
-- app_current_user_orgs() "my orgs" read (0003). See ADR-014.

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invitations
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
