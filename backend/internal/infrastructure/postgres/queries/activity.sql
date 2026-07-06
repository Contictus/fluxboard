-- Task activity log ([T], tenant-scoped, FR-TASK-002) -----------------------

-- name: AppendActivity :exec
INSERT INTO task_activity (id, org_id, task_id, actor_id, field, old_value, new_value)
VALUES (@id, @org_id, @task_id, @actor_id, @field,
        sqlc.narg('old_value'), sqlc.narg('new_value'));

-- name: ListActivityByTask :many
SELECT id, org_id, task_id, actor_id, field, old_value, new_value, created_at
FROM task_activity
WHERE org_id = @org_id AND task_id = @task_id
ORDER BY created_at DESC;
