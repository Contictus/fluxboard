-- 0032_ai_caller rollback.
DROP INDEX IF EXISTS ai_runs_key_idx;
ALTER TABLE ai_runs DROP COLUMN IF EXISTS key_id;
ALTER TABLE ai_runs ALTER COLUMN user_id SET NOT NULL;
