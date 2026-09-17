-- 0021_sprints -- Sprints (docs/08, FR-SPRINT). A sprint is a timeboxed
-- iteration in a project; tasks join via tasks.sprint_id (nullable: backlog).
-- Only one sprint per project may be active; the usecase enforces it.
-- completed_total/completed_done snapshot scope at completion for velocity.
-- DDL only; RLS lands in 0022 (the DDL/RLS split).
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE sprints (                             -- [T]
  id              uuid PRIMARY KEY,
  org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name            text NOT NULL,
  goal            text NOT NULL DEFAULT '',
  status          text NOT NULL DEFAULT 'planned', -- planned | active | completed
  started_at      timestamptz,
  ended_at        timestamptz,
  completed_total int NOT NULL DEFAULT 0,
  completed_done  int NOT NULL DEFAULT 0,
  created_by      uuid NOT NULL REFERENCES users(id),
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sprints_project_idx ON sprints (org_id, project_id, created_at DESC);

-- Backlog = tasks.sprint_id IS NULL. Existing rows default to backlog.
ALTER TABLE tasks ADD COLUMN sprint_id uuid REFERENCES sprints(id) ON DELETE SET NULL;
CREATE INDEX tasks_sprint_idx ON tasks (org_id, project_id, sprint_id) WHERE deleted_at IS NULL;
