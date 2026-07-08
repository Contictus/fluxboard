-- 0016_admin_rls — Row-Level Security for the Phase 6 tenant-owned tables. Same
-- pattern as 0004/0008/0010/0012/0014: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC ⇒ zero rows
-- (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS and never owns
-- these tables. audit_log is intentionally NOT covered here — it is not tenant-scoped
-- (nullable org_id, platform-level rows) and its viewer filters org_id in SQL; its
-- append-only guarantee is a grant (0005), not RLS.

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_keys
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE feature_flags ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON feature_flags
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE entitlement_overrides ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entitlement_overrides
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
