-- 0014 down — drop the Phase 5 RLS policies (tables dropped by 0013 down).
DROP POLICY IF EXISTS tenant_isolation ON project_stats_daily;
DROP POLICY IF EXISTS tenant_isolation ON notification_prefs;
DROP POLICY IF EXISTS tenant_isolation ON notifications;
