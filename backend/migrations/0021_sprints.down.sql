-- 0021_sprints rollback.
DROP INDEX IF EXISTS tasks_sprint_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS sprint_id;
DROP INDEX IF EXISTS sprints_project_idx;
DROP TABLE IF EXISTS sprints;
