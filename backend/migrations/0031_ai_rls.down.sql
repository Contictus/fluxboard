-- 0031_ai_rls rollback.
DROP POLICY IF EXISTS tenant_isolation ON ai_risks;
ALTER TABLE ai_risks DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON ai_runs;
ALTER TABLE ai_runs DISABLE ROW LEVEL SECURITY;
