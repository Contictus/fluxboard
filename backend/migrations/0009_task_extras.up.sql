-- 0009_task_extras — Phase 3b (docs/01 §TASK FR-TASK-006/007/009). Adds three
-- capabilities on top of the 3a task model:
--   * soft-delete / Trash (deleted_at) with a 30-day retention purge job,
--   * full-text search (generated tsvector + GIN over title/description),
--   * attachments (MinIO-backed; rows track the object lifecycle).
-- Bulk actions (FR-TASK-008) need no schema — they batch existing writes.
-- RLS for the new attachments table lands in 0010 (DDL/RLS split, as 0007/0008).

-- Soft-delete / Trash (FR-TASK-009) -----------------------------------------
-- A trashed task keeps its row (and its (column_id, rank) slot) so restore is a
-- single UPDATE. List/board/search all filter deleted_at IS NULL; the nightly
-- purge job hard-deletes rows past the retention window.
ALTER TABLE tasks ADD COLUMN deleted_at timestamptz;
CREATE INDEX tasks_trash_idx ON tasks (org_id, deleted_at) WHERE deleted_at IS NOT NULL;

-- Full-text search (FR-TASK-007) --------------------------------------------
-- Generated column keeps the tsvector in lockstep with title/description with no
-- application code or trigger; GIN index backs the @@ query.
ALTER TABLE tasks ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(description, '')), 'B')
  ) STORED;
CREATE INDEX tasks_search_idx ON tasks USING GIN (search_vector);

-- Attachments (FR-TASK-006) -------------------------------------------------
-- Two-phase lifecycle: a 'pending' row is created alongside a presigned PUT URL;
-- 'confirm' flips it to 'committed' after a HEAD verifies the object. Orphaned
-- 'pending' rows (client never PUT, or never confirmed) are GC'd nightly and
-- their objects removed. object_key is globally unique (the MinIO key).
CREATE TABLE attachments (                          -- [T]
  id           uuid PRIMARY KEY,
  org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id      uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  uploader_id  uuid NOT NULL REFERENCES users(id),
  object_key   text NOT NULL UNIQUE,                -- MinIO object key
  filename     text NOT NULL,
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
  status       text NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending', 'committed')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  confirmed_at timestamptz
);
CREATE INDEX attachments_task_idx ON attachments (task_id) WHERE status = 'committed';
CREATE INDEX attachments_gc_idx ON attachments (created_at) WHERE status = 'pending';
