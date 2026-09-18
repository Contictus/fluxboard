-- 0030_ai -- Governed AI layer tables (ADR-025, FR-AI-001..008). Append-only
-- run ledger (ai_runs) + dismissable risk signals (ai_risks). Both [T]:
-- every row carries org_id; RLS lands in 0031 (the DDL/RLS split).
-- Money = integer minor units where money appears (tokens are counts, not
-- money). Times = timestamptz UTC. Raw-pgx repos (audit_repo.go precedent);
-- no sqlc queries in this phase, so gen/ stays untouched and `sqlc diff` clean.

CREATE TABLE ai_runs (                            -- [T]
  id              uuid PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id         uuid NOT NULL REFERENCES users(id),
  kind            text NOT NULL, -- parse | plan_draft | digest | chat | risk
  input           text NOT NULL DEFAULT '',
  output          jsonb NOT NULL DEFAULT '{}',
  model           text NOT NULL DEFAULT 'mock',
  prompt_tokens   int NOT NULL DEFAULT 0,
  completion_tokens int NOT NULL DEFAULT 0,
  status          text NOT NULL DEFAULT 'ok', -- ok | error
  idempotency_key text,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_runs_org_idx ON ai_runs (org_id, created_at DESC);
-- Idempotency backstop per (org, key): NULL keys stay distinct in Postgres,
-- so only keyed writes dedupe. Same 24h Redis store as checkout/invite is the
-- primary guard; this UNIQUE is the crash-safe second layer.
CREATE UNIQUE INDEX ai_runs_idem_idx ON ai_runs (org_id, idempotency_key);

CREATE TABLE ai_risks (                            -- [T]
  id           uuid PRIMARY KEY,
  org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  task_id      uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  score        text NOT NULL DEFAULT 'low', -- low | medium | high
  signals      jsonb NOT NULL DEFAULT '[]',
  dismissed_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_risks_task_idx ON ai_risks (org_id, project_id, task_id);
-- One OPEN risk per task: recompute updates the row instead of re-noising
-- the team (FR-AI-004). Dismissed rows leave the partial index, so a later
-- recompute opens a fresh row.
CREATE UNIQUE INDEX ai_risks_open_idx ON ai_risks (org_id, task_id)
  WHERE dismissed_at IS NULL;
