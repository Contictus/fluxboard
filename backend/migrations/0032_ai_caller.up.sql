-- 0032_ai_caller -- Attribute AI runs to API-key callers (FR-AI-007). Session
-- callers persist user_id (FK users); key principals ("apikey:<id>", non-uuid,
-- see the Phase-6 nil-uuid fallback note) persist key_id instead. Either side
-- may be NULL, never both: the repo splits the caller before insert.

ALTER TABLE ai_runs ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE ai_runs ADD COLUMN key_id text;
CREATE INDEX ai_runs_key_idx ON ai_runs (org_id, key_id) WHERE key_id IS NOT NULL;
