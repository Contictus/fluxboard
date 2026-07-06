-- 0007_projects — Phase 3a core domain (docs/01 §PROJ/§TASK, docs/07 §3).
-- Projects, their default board + columns, tasks, subtasks, labels, comments and
-- the per-task activity log. All are tenant-owned ([T]); each carries an explicit
-- org_id (denormalized tenant key) so the uniform RLS predicate in 0008 applies
-- and every repo goes through TenantPool.WithTenant. Ordering columns (rank) hold
-- LexoRank-style keys minted by internal/pkg/rank (ADR-009): a board move touches
-- exactly one row.
--
-- Attachments (MinIO), full-text search, bulk actions and soft-delete/Trash are
-- Phase 3b and are NOT in this migration.

-- Projects ------------------------------------------------------------------
CREATE TABLE projects (                            -- [T]
  id            uuid PRIMARY KEY,
  org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  key           text NOT NULL CHECK (key ~ '^[A-Z]{2,6}$'), -- e.g. PAY (FR-PROJ-001)
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',           -- markdown
  color         text NOT NULL DEFAULT '',
  visibility    text NOT NULL DEFAULT 'org'
                CHECK (visibility IN ('org','private')),   -- FR-PROJ-002
  archived_at   timestamptz,                        -- FR-PROJ-006 (read-only when set)
  task_counter  int NOT NULL DEFAULT 0,             -- per-project task number source (FR-TASK-001)
  created_by    uuid NOT NULL REFERENCES users(id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, key)                              -- key unique per org
);
CREATE INDEX projects_org_idx ON projects (org_id, created_at DESC);
CREATE TRIGGER projects_set_updated_at BEFORE UPDATE ON projects
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Project membership + project roles (FR-PROJ-003). Org ADMIN+ have implicit
-- LEAD (enforced in the usecase layer, not stored here).
CREATE TABLE project_members (                     -- [T]
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       text NOT NULL CHECK (role IN ('LEAD','CONTRIBUTOR','VIEWER')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_members_user_idx ON project_members (org_id, user_id);

-- Boards + columns ----------------------------------------------------------
-- One default board per project in 3a (FR-PROJ-004); UNIQUE(project_id) holds
-- that invariant. Multi-board is a later concern.
CREATE TABLE boards (                              -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name       text NOT NULL DEFAULT 'Board',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id)
);
CREATE TRIGGER boards_set_updated_at BEFORE UPDATE ON boards
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE board_columns (                       -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  board_id   uuid NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name       text NOT NULL,
  rank       text NOT NULL,                         -- LexoRank ordering (ADR-009)
  wip_limit  int,                                   -- optional soft cap (FR-PROJ-004)
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX board_columns_board_idx ON board_columns (board_id, rank);
CREATE TRIGGER board_columns_set_updated_at BEFORE UPDATE ON board_columns
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Tasks ---------------------------------------------------------------------
CREATE TABLE tasks (                               -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  column_id   uuid NOT NULL REFERENCES board_columns(id),
  number      int NOT NULL,                         -- PAY-123, per-project (FR-TASK-001)
  title       text NOT NULL,
  description text NOT NULL DEFAULT '',             -- markdown
  assignee_id uuid REFERENCES users(id),
  priority    text NOT NULL DEFAULT 'none'
              CHECK (priority IN ('urgent','high','medium','low','none')),
  due_date    timestamptz,
  rank        text NOT NULL,                         -- LexoRank within its column
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, number),
  UNIQUE (column_id, rank)                          -- two tasks never share a rank in a column;
);                                                  -- a concurrent identical move 409s (FR-PROJ-005)
CREATE INDEX tasks_column_idx ON tasks (column_id, rank);
CREATE INDEX tasks_project_idx ON tasks (project_id);
CREATE TRIGGER tasks_set_updated_at BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Subtasks: flat one-level checklist (FR-TASK-003).
CREATE TABLE subtasks (                            -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  title      text NOT NULL,
  done       boolean NOT NULL DEFAULT false,
  rank       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX subtasks_task_idx ON subtasks (task_id, rank);
CREATE TRIGGER subtasks_set_updated_at BEFORE UPDATE ON subtasks
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Labels: org-scoped, reusable across tasks (FR-TASK-004).
CREATE TABLE labels (                              -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name       text NOT NULL CHECK (char_length(name) <= 30),
  color      text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);
CREATE TRIGGER labels_set_updated_at BEFORE UPDATE ON labels
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- task_labels: many-to-many; ON DELETE CASCADE from labels detaches everywhere.
CREATE TABLE task_labels (                         -- [T]
  task_id  uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  label_id uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  org_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  PRIMARY KEY (task_id, label_id)
);
CREATE INDEX task_labels_label_idx ON task_labels (label_id);

-- Comments: markdown, 15-min edit window + soft delete (FR-TASK-005).
CREATE TABLE comments (                            -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  author_id  uuid NOT NULL REFERENCES users(id),
  body       text NOT NULL,
  edited     boolean NOT NULL DEFAULT false,
  deleted_at timestamptz,                           -- soft delete placeholder
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX comments_task_idx ON comments (task_id, created_at);
CREATE TRIGGER comments_set_updated_at BEFORE UPDATE ON comments
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Per-task activity log: every field change (FR-TASK-002).
CREATE TABLE task_activity (                       -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  actor_id   uuid NOT NULL REFERENCES users(id),
  field      text NOT NULL,
  old_value  text,
  new_value  text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX task_activity_task_idx ON task_activity (task_id, created_at DESC);
