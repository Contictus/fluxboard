DROP POLICY IF EXISTS tenant_isolation ON entitlement_overrides;
ALTER TABLE entitlement_overrides DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON feature_flags;
ALTER TABLE feature_flags DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON api_keys;
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;
