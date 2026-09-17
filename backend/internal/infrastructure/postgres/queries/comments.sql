-- Comments ([T], tenant-scoped, FR-TASK-005) --------------------------------

-- name: CreateComment :exec
INSERT INTO comments (id, org_id, task_id, author_id, body)
VALUES (@id, @org_id, @task_id, @author_id, @body);

-- name: GetComment :one
SELECT id, org_id, task_id, author_id, body, edited, deleted_at, created_at, updated_at
FROM comments
WHERE org_id = @org_id AND id = @id;

-- name: ListCommentsByTask :many
SELECT id, org_id, task_id, author_id, body, edited, deleted_at, created_at, updated_at
FROM comments
WHERE org_id = @org_id AND task_id = @task_id
ORDER BY created_at;

-- name: UpdateComment :execrows
UPDATE comments SET body = @body, edited = true
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- name: SoftDeleteComment :execrows
UPDATE comments SET deleted_at = now()
WHERE org_id = @org_id AND id = @id AND deleted_at IS NULL;

-- name: CountCommentsForProject :many
SELECT c.task_id, COUNT(*) AS total
FROM comments c
JOIN tasks t ON t.id = c.task_id
WHERE c.org_id = @org_id AND t.project_id = @project_id AND c.deleted_at IS NULL
GROUP BY c.task_id;
