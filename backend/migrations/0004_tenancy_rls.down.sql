DROP POLICY IF EXISTS tenant_isolation ON invitations;
ALTER TABLE invitations DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON memberships;
ALTER TABLE memberships DISABLE ROW LEVEL SECURITY;
