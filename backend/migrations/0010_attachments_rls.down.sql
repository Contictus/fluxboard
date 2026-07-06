DROP POLICY IF EXISTS tenant_isolation ON attachments;
ALTER TABLE attachments DISABLE ROW LEVEL SECURITY;
