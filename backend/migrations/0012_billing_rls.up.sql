-- 0012_billing_rls — Row-Level Security for the Phase 4 tenant-owned billing
-- tables. Same pattern as 0004/0008/0010: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC ⇒ zero rows
-- (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS and never owns
-- these tables.
--
-- NOT isolated here (deliberately global): plans (reference data) and
-- processed_stripe_events (written from the unauthenticated webhook endpoint,
-- which has no tenant context). See 0011.

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscriptions
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoices
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE usage_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON usage_records
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outbox
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
