-- 0023_custom_fields rollback.
DROP INDEX IF EXISTS task_custom_values_task_idx;
DROP TABLE IF EXISTS task_custom_values;
DROP INDEX IF EXISTS custom_fields_project_idx;
DROP TABLE IF EXISTS custom_fields;
