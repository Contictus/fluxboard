-- 0023_custom_fields -- Custom fields (docs/08, FR-FIELDS). Project-scoped
-- field definitions plus per-task values. Types: text | number | date |
-- select (options JSON array of strings). Values live in typed nullable
-- columns so numbers and dates sort and validate natively; text covers
-- select choices. Deleting a field cascades its values.
-- DDL only; RLS lands in 0024 (the DDL/RLS split).
-- Times = timestamptz UTC. Every tenant-owned table carries org_id for RLS.

CREATE TABLE custom_fields (                        -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name        text NOT NULL,
  type        text NOT NULL,                        -- text | number | date | select
  options     jsonb NOT NULL DEFAULT '[]',          -- select choices
  position    int NOT NULL DEFAULT 0,
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX custom_fields_project_idx ON custom_fields (org_id, project_id, position);

CREATE TABLE task_custom_values (                    -- [T]
  task_id     uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  field_id    uuid NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  value_text  text,
  value_number numeric,
  value_date  timestamptz,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (task_id, field_id)
);
CREATE INDEX task_custom_values_task_idx ON task_custom_values (org_id, task_id);
