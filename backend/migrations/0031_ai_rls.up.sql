-- 0031_ai_rls -- Row-Level Security for the AI tables. Same pattern as
-- 0004/0008/0010/0012/0014/0016/0018/0020/0022/0024/0026/0028: predicate on the
-- per-transaction GUC app.current_tenant set via SET LOCAL by TenantPool.
-- Unset GUC means zero rows (fail-closed). ENABLE (not FORCE): the app role is
-- NOBYPASSRLS and never owns these tables, so ENABLE fully isolates it (ADR-014).

ALTER TABLE ai_runs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_runs
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE ai_risks ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_risks
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
