-- 0018_automation_rls -- Row-Level Security for automation_rules. Same
-- pattern as 0004/0008/0010/0012/0014/0016: predicate on the per-transaction
-- GUC app.current_tenant set via SET LOCAL by TenantPool. Unset GUC means
-- zero rows (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS
-- and never owns these tables.

ALTER TABLE automation_rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON automation_rules
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
