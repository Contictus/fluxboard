-- 0026_task_links_rls -- Row-Level Security for task_links. Same pattern as
-- earlier RLS migrations: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC means zero
-- rows (fail-closed). ENABLE (not FORCE).

ALTER TABLE task_links ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_links
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
