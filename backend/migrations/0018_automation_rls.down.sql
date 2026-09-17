-- 0018_automation_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON automation_rules;
ALTER TABLE automation_rules DISABLE ROW LEVEL SECURITY;
