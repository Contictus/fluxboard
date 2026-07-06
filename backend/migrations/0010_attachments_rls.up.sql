-- 0010_attachments_rls — Row-Level Security for the Phase 3b attachments table.
-- Same pattern as 0008/0004: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC ⇒ zero rows
-- (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS and never owns
-- the table. Maintenance jobs (orphan GC) run per-tenant through TenantPool.

ALTER TABLE attachments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attachments
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
