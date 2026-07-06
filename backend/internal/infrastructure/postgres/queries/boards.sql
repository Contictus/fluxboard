-- Boards + columns ([T], tenant-scoped, FR-PROJ-004) -----------------------

-- name: CreateBoard :exec
INSERT INTO boards (id, org_id, project_id, name)
VALUES (@id, @org_id, @project_id, @name);

-- name: GetBoardByProject :one
SELECT id, org_id, project_id, name, created_at, updated_at
FROM boards
WHERE org_id = @org_id AND project_id = @project_id;

-- name: CreateColumn :exec
INSERT INTO board_columns (id, org_id, board_id, name, rank, wip_limit)
VALUES (@id, @org_id, @board_id, @name, @rank, sqlc.narg('wip_limit'));

-- name: GetColumn :one
SELECT id, org_id, board_id, name, rank, wip_limit, created_at, updated_at
FROM board_columns
WHERE org_id = @org_id AND id = @id;

-- name: ListColumnsByBoard :many
SELECT id, org_id, board_id, name, rank, wip_limit, created_at, updated_at
FROM board_columns
WHERE org_id = @org_id AND board_id = @board_id
ORDER BY rank;

-- name: UpdateColumn :execrows
UPDATE board_columns
SET name = @name, wip_limit = sqlc.narg('wip_limit')
WHERE org_id = @org_id AND id = @id;

-- name: SetColumnRank :execrows
UPDATE board_columns SET rank = @rank
WHERE org_id = @org_id AND id = @id;

-- name: DeleteColumn :execrows
DELETE FROM board_columns
WHERE org_id = @org_id AND id = @id;

-- name: CountColumnTasks :one
SELECT count(*) FROM tasks
WHERE org_id = @org_id AND column_id = @column_id;
