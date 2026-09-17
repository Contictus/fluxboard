-- 0019_time_tracking -- Time entries (docs/08, FR-TIME). One row per timer
-- run or manual entry: ended_at NULL means the timer is running. Only one
-- running entry per (org, user) is enforced by the application (starting a
-- new timer stops the previous one), so no partial unique index is needed.
-- DDL only; RLS lands in 0020 (the DDL/RLS split).
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE time_entries (                         -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id     uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  started_at  timestamptz NOT NULL,
  ended_at    timestamptz,
  note        text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- Task detail timeline + running-timer lookup both hit these.
CREATE INDEX time_entries_task_idx ON time_entries (org_id, task_id, started_at DESC);
CREATE INDEX time_entries_running_idx ON time_entries (org_id, user_id) WHERE ended_at IS NULL;
