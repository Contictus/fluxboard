-- 0022_sprints_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON sprints;
ALTER TABLE sprints DISABLE ROW LEVEL SECURITY;
