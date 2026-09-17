-- 0028_project_forms_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON project_forms;
ALTER TABLE project_forms DISABLE ROW LEVEL SECURITY;
