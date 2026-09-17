-- 0024_custom_fields_rls -- Row-Level Security for custom_fields and
-- task_custom_values. Same pattern as earlier RLS migrations: predicate on
-- the per-transaction GUC app.current_tenant set via SET LOCAL by
-- TenantPool. Unset GUC means zero rows (fail-closed). ENABLE (not FORCE).

ALTER TABLE custom_fields ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON custom_fields
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE task_custom_values ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_custom_values
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
