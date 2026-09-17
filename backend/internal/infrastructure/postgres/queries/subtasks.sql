-- Subtasks ([T], tenant-scoped, FR-TASK-003) --------------------------------

-- name: CreateSubtask :exec
INSERT INTO subtasks (id, org_id, task_id, title, done, rank)
VALUES (@id, @org_id, @task_id, @title, @done, @rank);

-- name: GetSubtask :one
SELECT id, org_id, task_id, title, done, rank, created_at, updated_at
FROM subtasks
WHERE org_id = @org_id AND id = @id;

-- name: ListSubtasksByTask :many
SELECT id, org_id, task_id, title, done, rank, created_at, updated_at
FROM subtasks
WHERE org_id = @org_id AND task_id = @task_id
ORDER BY rank;

-- name: UpdateSubtask :execrows
UPDATE subtasks SET title = @title, done = @done
WHERE org_id = @org_id AND id = @id;

-- name: DeleteSubtask :execrows
DELETE FROM subtasks WHERE org_id = @org_id AND id = @id;

-- name: CountSubtasksForProject :many
SELECT s.task_id, COUNT(*) AS total, COUNT(*) FILTER (WHERE s.done) AS done
FROM subtasks s
JOIN tasks t ON t.id = s.task_id
WHERE s.org_id = @org_id AND t.project_id = @project_id
GROUP BY s.task_id;
