-- 0024_custom_fields_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON task_custom_values;
ALTER TABLE task_custom_values DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON custom_fields;
ALTER TABLE custom_fields DISABLE ROW LEVEL SECURITY;
