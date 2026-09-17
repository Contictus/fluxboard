-- Intake forms ([T], tenant-scoped, ADR-023) ----------------------------------
-- Token resolution (public surface) intentionally lives OUTSIDE sqlc: it calls
-- the SECURITY DEFINER app_form_by_token function whose RETURNS TABLE columns
-- sqlc can't infer (same as app_invitation_by_token — hand-written pgx in the
-- form repo).

-- name: CreateProjectForm :exec
INSERT INTO project_forms (id, org_id, project_id, name, description, target_column_id, token_hash, created_by)
VALUES (@id, @org_id, @project_id, @name, @description, @target_column_id, @token_hash, @created_by);

-- name: GetProjectForm :one
SELECT id, org_id, project_id, name, description, target_column_id, token_hash,
       created_by, is_active, created_at, updated_at
FROM project_forms
WHERE org_id = @org_id AND id = @id;

-- name: ListProjectFormsByProject :many
SELECT id, org_id, project_id, name, description, target_column_id, token_hash,
       created_by, is_active, created_at, updated_at
FROM project_forms
WHERE org_id = @org_id AND project_id = @project_id
ORDER BY created_at;

-- name: UpdateProjectForm :execrows
UPDATE project_forms
SET name = @name, description = @description, target_column_id = @target_column_id,
    is_active = @is_active, updated_at = now()
WHERE org_id = @org_id AND id = @id;

-- name: UpdateProjectFormToken :execrows
UPDATE project_forms
SET token_hash = @token_hash, updated_at = now()
WHERE org_id = @org_id AND id = @id;

-- name: DeleteProjectForm :execrows
DELETE FROM project_forms
WHERE org_id = @org_id AND id = @id;
