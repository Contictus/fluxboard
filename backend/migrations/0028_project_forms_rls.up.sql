-- 0028_project_forms_rls -- Row-Level Security for project_forms. Same pattern
-- as earlier RLS migrations: predicate on the per-transaction GUC
-- app.current_tenant set via SET LOCAL by TenantPool. Unset GUC means zero
-- rows (fail-closed). ENABLE (not FORCE) so the SECURITY DEFINER
-- app_form_by_token read keeps working (ADR-014/023).

ALTER TABLE project_forms ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_forms
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
