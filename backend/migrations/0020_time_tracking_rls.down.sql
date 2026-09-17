-- 0020_time_tracking_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON time_entries;
ALTER TABLE time_entries DISABLE ROW LEVEL SECURITY;
