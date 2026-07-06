-- Tasks ([T], tenant-scoped) ------------------------------------------------

-- name: CreateTask :exec
INSERT INTO tasks (id, org_id, project_id, column_id, number, title, description,
                   assignee_id, priority, due_date, rank, created_by)
VALUES (@id, @org_id, @project_id, @column_id, @number, @title, @description,
        sqlc.narg('assignee_id'), @priority, sqlc.narg('due_date'), @rank, @created_by);

-- name: GetTask :one
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, due_date, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND id = @id;

-- name: ListTasksByColumn :many
SELECT id, org_id, project_id, column_id, number, title, description,
       assignee_id, priority, due_date, rank, created_by, created_at, updated_at
FROM tasks
WHERE org_id = @org_id AND column_id = @column_id
ORDER BY rank;

-- name: ListTasksByProject :many
SELECT t.id, t.org_id, t.project_id, t.column_id, t.number, t.title, t.description,
       t.assignee_id, t.priority, t.due_date, t.rank, t.created_by, t.created_at, t.updated_at
FROM tasks t
JOIN board_columns c ON c.id = t.column_id
WHERE t.org_id = @org_id AND t.project_id = @project_id
ORDER BY c.rank, t.rank;

-- name: UpdateTask :execrows
UPDATE tasks
SET title = @title, description = @description,
    assignee_id = sqlc.narg('assignee_id'), priority = @priority,
    due_date = sqlc.narg('due_date')
WHERE org_id = @org_id AND id = @id;

-- name: MoveTask :execrows
-- Relocate a task. The (column_id, rank) unique index makes a concurrent
-- identical move raise a unique violation → domain.ErrConflict → 409 (FR-PROJ-005).
UPDATE tasks SET column_id = @column_id, rank = @rank
WHERE org_id = @org_id AND id = @id;
