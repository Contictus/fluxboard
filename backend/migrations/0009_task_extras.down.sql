-- Reverse 0009 in dependency order.
DROP TABLE IF EXISTS attachments;
DROP INDEX IF EXISTS tasks_search_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS search_vector;
DROP INDEX IF EXISTS tasks_trash_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS deleted_at;
