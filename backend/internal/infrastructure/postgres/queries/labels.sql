-- Labels + task attachments ([T], tenant-scoped, FR-TASK-004) ---------------

-- name: CreateLabel :exec
INSERT INTO labels (id, org_id, name, color)
VALUES (@id, @org_id, @name, @color);

-- name: GetLabel :one
SELECT id, org_id, name, color, created_at, updated_at
FROM labels
WHERE org_id = @org_id AND id = @id;

-- name: ListLabels :many
SELECT id, org_id, name, color, created_at, updated_at
FROM labels
WHERE org_id = @org_id
ORDER BY name;

-- name: UpdateLabel :execrows
UPDATE labels SET name = @name, color = @color
WHERE org_id = @org_id AND id = @id;

-- name: DeleteLabel :execrows
-- The FK cascade on task_labels detaches this label from every task.
DELETE FROM labels WHERE org_id = @org_id AND id = @id;

-- name: AttachLabel :exec
INSERT INTO task_labels (task_id, label_id, org_id)
VALUES (@task_id, @label_id, @org_id)
ON CONFLICT (task_id, label_id) DO NOTHING;

-- name: DetachLabel :execrows
DELETE FROM task_labels
WHERE org_id = @org_id AND task_id = @task_id AND label_id = @label_id;

-- name: ListLabelsForTask :many
SELECT l.id, l.org_id, l.name, l.color, l.created_at, l.updated_at
FROM task_labels tl
JOIN labels l ON l.id = tl.label_id
WHERE tl.org_id = @org_id AND tl.task_id = @task_id
ORDER BY l.name;

-- name: ListLabelsForProject :many
SELECT tl.task_id, l.id, l.org_id, l.name, l.color, l.created_at, l.updated_at
FROM task_labels tl
JOIN labels l ON l.id = tl.label_id
JOIN tasks t ON t.id = tl.task_id
WHERE tl.org_id = @org_id AND t.project_id = @project_id
ORDER BY l.name;
