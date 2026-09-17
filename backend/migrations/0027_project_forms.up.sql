-- 0027_project_forms -- Public intake forms (ADR-023). One row per shareable
-- form: renders at /f/<token> and files submitted tasks into target_column_id.
-- Only token_hash is stored (pkg/token SHA-256, UNIQUE); the raw token is shown
-- once at create/rotate. target_column_id intentionally carries NO foreign key
-- (RESTRICT would break column deletion, CASCADE would silently kill the form);
-- the submit usecase validates column-in-project instead. created_by attributes
-- anonymous submissions (tasks.created_by is NOT NULL + FK to users).
-- DDL only; RLS lands in 0028 (the DDL/RLS split).
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE project_forms (                        -- [T]
  id               uuid NOT NULL PRIMARY KEY,
  org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id       uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name             text NOT NULL,
  description      text NOT NULL DEFAULT '',
  target_column_id uuid NOT NULL,
  token_hash       bytea NOT NULL UNIQUE,
  created_by       uuid NOT NULL REFERENCES users(id),
  is_active        boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_forms_project_idx ON project_forms (org_id, project_id);

-- app_form_by_token — sanctioned cross-tenant read for the public form page
-- and the anonymous submit flow. The org is unknown until the row is read and
-- project_forms is RLS-isolated, so this SECURITY DEFINER function (same
-- pattern as app_invitation_by_token, ADR-014) resolves the narrow projection
-- the public surface needs. Returns at most one row (token_hash is UNIQUE).
-- Never exposes token_hash itself.
CREATE FUNCTION app_form_by_token(p_hash bytea)
RETURNS TABLE (
  id uuid, org_id uuid, project_id uuid, name text, description text,
  target_column_id uuid, created_by uuid, is_active boolean
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT f.id, f.org_id, f.project_id, f.name, f.description,
         f.target_column_id, f.created_by, f.is_active
  FROM project_forms f
  WHERE f.token_hash = p_hash;
$$;
REVOKE ALL ON FUNCTION app_form_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_form_by_token(bytea) TO fluxboard_app;
