-- 0025_task_links rollback.
DROP INDEX IF EXISTS task_links_linked_idx;
DROP TABLE IF EXISTS task_links;
