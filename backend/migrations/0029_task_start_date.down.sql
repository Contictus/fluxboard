-- 0029_task_start_date rollback.
ALTER TABLE tasks DROP COLUMN IF EXISTS start_date;
