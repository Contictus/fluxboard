-- 0022_sprints_rls -- Row-Level Security for sprints. Same pattern as
-- 0004/0008/0010/0012/0014/0016/0018/0020: predicate on the per-transaction
-- GUC app.current_tenant set via SET LOCAL by TenantPool. Unset GUC means
-- zero rows (fail-closed). ENABLE (not FORCE). tasks is already covered.

ALTER TABLE sprints ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sprints
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
