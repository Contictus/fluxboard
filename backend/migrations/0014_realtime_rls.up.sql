-- 0014_realtime_rls — Row-Level Security for the Phase 5 tenant-owned tables. Same
-- pattern as 0004/0008/0010/0012: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC ⇒ zero rows
-- (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS and never owns
-- these tables.

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notifications
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE notification_prefs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_prefs
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE project_stats_daily ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_stats_daily
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
