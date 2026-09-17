-- Tasks ([T], tenant-scoped) ------------------------------------------------

-- name: CreateTask :exec
INSERT INTO tasks (id, org_id, project_id, column_id, number, title, description,
                   assignee_id, priority, start_date, due_date, rank, created_by)
VALUES (@id, @org_id, @project_id, @column_id, @number, @title, @description,
        sqlc.narg('assignee_id'), @priority, sqlc.narg('start_date'), sqlc.narg('due_date'), @rank, @created_by);

-- name: GetTask :one
-- Live tasks only; trashed tasks are addressable through the Trash queries.
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, start_date, due_date, sprint_id, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- name: ListTasksByColumn :many
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, start_date, due_date, sprint_id, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND column_id = @column_id AND deleted_at IS NULL
ORDER BY rank;

-- name: ListTasksByProject :many
SELECT t.id, t.org_id, t.project_id, t.column_id, t.number, t.title, t.description,
       t.assignee_id, t.priority, t.start_date, t.due_date, t.sprint_id, t.rank, t.created_by, t.created_at, t.updated_at
FROM tasks t
JOIN board_columns c ON c.id = t.column_id
WHERE t.org_id = @org_id AND t.project_id = @project_id AND t.deleted_at IS NULL
ORDER BY c.rank, t.rank;

-- name: UpdateTask :execrows
UPDATE tasks
SET title = @title, description = @description,
    assignee_id = sqlc.narg('assignee_id'), priority = @priority,
    start_date = sqlc.narg('start_date'), due_date = sqlc.narg('due_date')
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- name: MoveTask :execrows
-- Relocate a task. The (column_id, rank) unique index makes a concurrent
-- identical move raise a unique violation → domain.ErrConflict → 409 (FR-PROJ-005).
UPDATE tasks SET column_id = @column_id, rank = @rank
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- Soft-delete / Trash (FR-TASK-009) -----------------------------------------

-- name: SoftDeleteTask :execrows
UPDATE tasks SET deleted_at = @deleted_at
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- name: RestoreTask :execrows
-- Restore only succeeds if the original (column_id, rank) slot is still free;
-- the unique index otherwise raises a conflict the repo maps to 409.
UPDATE tasks SET deleted_at = NULL
WHERE org_id = @org_id AND id = @id AND deleted_at IS NOT NULL;

-- name: GetTrashedTask :one
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, start_date, due_date, sprint_id, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND id = @id AND deleted_at IS NOT NULL;

-- name: ListTrashedTasks :many
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, start_date, due_date, sprint_id, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND project_id = @project_id AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC;

-- name: PurgeExpiredTrash :execrows
-- Hard-delete tasks trashed before the cutoff (nightly purge job, per-tenant).
DELETE FROM tasks
WHERE org_id = @org_id AND deleted_at IS NOT NULL AND deleted_at < @cutoff;

-- Full-text search (FR-TASK-007) --------------------------------------------

-- name: SearchTasks :many
-- Org-wide task search over title+description with optional facet filters.
-- Visibility is enforced in-DB: a caller sees a task only if it is in an 'org'
-- project, or the caller is an org admin (@see_all), or the caller is a member of
-- the (private) project. Archived projects are excluded. count(*) OVER() yields
-- the unpaged total for pagination; the label filter pins one label_id per row so
-- DISTINCT is unnecessary.
SELECT t.id, t.org_id, t.project_id, t.column_id, t.number, t.title, t.description,
       t.assignee_id, t.priority, t.start_date, t.due_date, t.sprint_id, t.rank, t.created_by,
       t.created_at, t.updated_at,
       count(*) OVER() AS total_count
FROM tasks t
JOIN projects p ON p.id = t.project_id
LEFT JOIN task_labels tl ON tl.task_id = t.id AND tl.org_id = t.org_id
WHERE t.org_id = @org_id
  AND t.deleted_at IS NULL
  AND p.archived_at IS NULL
  AND t.search_vector @@ websearch_to_tsquery('simple', @query)
  AND (@see_all::bool OR p.visibility = 'org' OR EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.project_id = p.id AND pm.user_id = @user_id))
  AND (sqlc.narg('project_id')::uuid IS NULL OR t.project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR t.assignee_id = sqlc.narg('assignee_id'))
  AND (sqlc.narg('column_id')::uuid IS NULL OR t.column_id = sqlc.narg('column_id'))
  AND (sqlc.narg('priority')::text IS NULL OR t.priority = sqlc.narg('priority'))
  AND (sqlc.narg('label_id')::uuid IS NULL OR tl.label_id = sqlc.narg('label_id'))
ORDER BY ts_rank(t.search_vector, websearch_to_tsquery('simple', @query)) DESC,
         t.created_at DESC
LIMIT @lim OFFSET @off;

-- Bulk actions (FR-TASK-008) ------------------------------------------------
-- Each runs inside one repo transaction over a task-id array; RLS + org_id scope
-- every row. Bulk move relocates to a single column but must give each task a
-- distinct rank, so the repo issues per-task MoveTask calls instead of a set-based
-- UPDATE (the (column_id, rank) unique index forbids sharing a rank).

-- name: BulkAssignTasks :execrows
UPDATE tasks SET assignee_id = sqlc.narg('assignee_id')
WHERE org_id = @org_id AND deleted_at IS NULL AND id = ANY(@ids::uuid[]);

-- name: ValidateTaskIDs :one
-- Count how many of the given ids are live tasks in this org (bulk pre-check).
SELECT count(*) FROM tasks
WHERE org_id = @org_id AND deleted_at IS NULL AND id = ANY(@ids::uuid[]);
