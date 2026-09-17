-- Task links ([T], tenant-scoped, FR-LINKS) ----------------------------------

-- name: CreateTaskLink :exec
INSERT INTO task_links (task_id, linked_task_id, org_id)
VALUES (@task_id, @linked_task_id, @org_id)
ON CONFLICT (task_id, linked_task_id) DO NOTHING;

-- name: DeleteTaskLink :execrows
DELETE FROM task_links
WHERE org_id = @org_id AND task_id = @task_id AND linked_task_id = @linked_task_id;

-- name: ListTaskBlockers :many
SELECT t.id, t.org_id, t.project_id, t.column_id, t.number, t.title, t.created_at
FROM task_links tl
JOIN tasks t ON t.id = tl.linked_task_id
WHERE tl.org_id = @org_id AND tl.task_id = @task_id AND t.deleted_at IS NULL
ORDER BY t.number;

-- name: ListTaskBlocked :many
SELECT t.id, t.org_id, t.project_id, t.column_id, t.number, t.title, t.created_at
FROM task_links tl
JOIN tasks t ON t.id = tl.task_id
WHERE tl.org_id = @org_id AND tl.linked_task_id = @task_id AND t.deleted_at IS NULL
ORDER BY t.number;
