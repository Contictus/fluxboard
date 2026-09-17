-- 0027_project_forms rollback.
DROP FUNCTION IF EXISTS app_form_by_token(bytea);
DROP INDEX IF EXISTS project_forms_project_idx;
DROP TABLE IF EXISTS project_forms;
