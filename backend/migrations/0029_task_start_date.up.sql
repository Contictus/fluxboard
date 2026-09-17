-- 0029_task_start_date -- Optional planned start for tasks. Pairs with due_date
-- so boards/timelines can show a real scheduled window instead of deriving the
-- bar start from created_at. NULL = unscheduled start (old rows stay NULL).
-- Plain column add on an RLS-isolated table: no policy change needed.

ALTER TABLE tasks ADD COLUMN start_date timestamptz;
