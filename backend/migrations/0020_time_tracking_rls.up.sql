-- 0020_time_tracking_rls -- Row-Level Security for time_entries. Same
-- pattern as 0004/0008/0010/0012/0014/0016/0018: predicate on the
-- per-transaction GUC app.current_tenant set via SET LOCAL by TenantPool.
-- Unset GUC means zero rows (fail-closed). ENABLE (not FORCE).

ALTER TABLE time_entries ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON time_entries
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
