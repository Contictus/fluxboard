-- Custom fields ([T], tenant-scoped, FR-FIELDS) ----------------------------

-- name: CreateCustomField :exec
INSERT INTO custom_fields (id, org_id, project_id, name, type, options, position, created_by)
VALUES (@id, @org_id, @project_id, @name, @type, @options, @position, @created_by);

-- name: GetCustomField :one
SELECT id, org_id, project_id, name, type, options, position, created_by, created_at
FROM custom_fields
WHERE org_id = @org_id AND id = @id;

-- name: ListCustomFieldsByProject :many
SELECT id, org_id, project_id, name, type, options, position, created_by, created_at
FROM custom_fields
WHERE org_id = @org_id AND project_id = @project_id
ORDER BY position, created_at;

-- name: UpdateCustomField :execrows
UPDATE custom_fields SET name = @name, options = @options, position = @position
WHERE org_id = @org_id AND id = @id;

-- name: DeleteCustomField :execrows
DELETE FROM custom_fields
WHERE org_id = @org_id AND id = @id;

-- name: UpsertTaskCustomValue :exec
INSERT INTO task_custom_values (task_id, field_id, org_id, value_text, value_number, value_date, updated_at)
VALUES (@task_id, @field_id, @org_id, @value_text, @value_number, @value_date, now())
ON CONFLICT (task_id, field_id) DO UPDATE SET
  value_text = EXCLUDED.value_text,
  value_number = EXCLUDED.value_number,
  value_date = EXCLUDED.value_date,
  updated_at = now();

-- name: ListTaskCustomValues :many
SELECT v.task_id, v.field_id, v.org_id, v.value_text, v.value_number, v.value_date, v.updated_at,
       f.name, f.type
FROM task_custom_values v
JOIN custom_fields f ON f.id = v.field_id
WHERE v.org_id = @org_id AND v.task_id = @task_id
ORDER BY f.position, f.created_at;

-- name: DeleteTaskCustomValue :execrows
DELETE FROM task_custom_values
WHERE org_id = @org_id AND task_id = @task_id AND field_id = @field_id;
