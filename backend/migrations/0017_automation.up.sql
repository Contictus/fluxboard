-- 0017_automation -- Automation rules (docs/08, FR-AUTO). Org-scoped
-- "when X happens, do Y" rules evaluated synchronously on the task write
-- path (best-effort, never fails the write). DDL only; RLS lands in 0018
-- (the DDL/RLS split, as 0011/0012 and 0015/0016).
--
-- Triggers (trigger): task.created | task.moved | task.assigned.
-- trigger_config carries optional conditions, e.g. {"column_id": "<uuid>"}
-- for task.moved (only when moved INTO that column).
-- Actions (action): assign | set_priority | move | add_label.
-- action_config carries the parameters, e.g. {"user_id": "<uuid>"},
-- {"priority": "high"}, {"column_id": "<uuid>"}, {"label_id": "<uuid>"}.
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE automation_rules (                    -- [T]
  id             uuid PRIMARY KEY,
  org_id         uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name           text NOT NULL,
  enabled        boolean NOT NULL DEFAULT true,
  trigger        text NOT NULL,                     -- task.created | task.moved | task.assigned
  trigger_config jsonb NOT NULL DEFAULT '{}',
  action         text NOT NULL,                     -- assign | set_priority | move | add_label
  action_config  jsonb NOT NULL DEFAULT '{}',
  created_by     uuid NOT NULL REFERENCES users(id),
  created_at     timestamptz NOT NULL DEFAULT now()
);
-- Rule listing per org (enabled-first) and the evaluate path both hit this.
CREATE INDEX automation_rules_org_idx ON automation_rules (org_id, enabled, trigger);
