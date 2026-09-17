-- 0019_time_tracking rollback.
DROP INDEX IF EXISTS time_entries_running_idx;
DROP INDEX IF EXISTS time_entries_task_idx;
DROP TABLE IF EXISTS time_entries;
