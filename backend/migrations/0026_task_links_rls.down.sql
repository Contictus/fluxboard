-- 0026_task_links_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON task_links;
ALTER TABLE task_links DISABLE ROW LEVEL SECURITY;
