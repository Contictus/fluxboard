-- 0025_task_links -- Task dependencies (docs/08, FR-LINKS). Directed edges:
-- (task_id) is blocked by (linked_task_id). Only the "blocks" relation in
-- v1; the inverse reads as "blocked by". Self-links are rejected by check
-- constraint; duplicates by primary key. Cycle detection is intentionally
-- left to a future version (documented in FR-LINKS).
-- DDL only; RLS lands in 0026 (the DDL/RLS split).
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE task_links (                             -- [T]
  task_id        uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  linked_task_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  org_id         uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (task_id, linked_task_id),
  CONSTRAINT task_links_no_self CHECK (task_id <> linked_task_id)
);
CREATE INDEX task_links_linked_idx ON task_links (org_id, linked_task_id);
