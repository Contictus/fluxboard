-- 0008_projects_rls — Row-Level Security for the Phase 3a tenant tables. Kept
-- separate from the DDL (docs/07 §5) so the isolation layer reviews as one
-- artifact. Same pattern as 0004: predicate on the per-transaction GUC
-- app.current_tenant, set via SET LOCAL by TenantPool. Unset GUC ⇒ NULL predicate
-- ⇒ zero rows (fail-closed). ENABLE (not FORCE): the app role is NOBYPASSRLS and
-- never owns these tables, so ENABLE fully isolates it. See ADR-014.

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON projects
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE project_members ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_members
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON boards
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE board_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON board_columns
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tasks
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE subtasks ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subtasks
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE labels ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON labels
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE task_labels ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_labels
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE comments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON comments
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE task_activity ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_activity
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
