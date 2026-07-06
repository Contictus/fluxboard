-- Project members ([T], tenant-scoped, FR-PROJ-003) ------------------------

-- name: AddProjectMember :exec
INSERT INTO project_members (project_id, org_id, user_id, role)
VALUES (@project_id, @org_id, @user_id, @role)
ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: GetProjectMember :one
SELECT project_id, user_id, role, created_at
FROM project_members
WHERE org_id = @org_id AND project_id = @project_id AND user_id = @user_id;

-- name: ListProjectMembers :many
SELECT project_id, user_id, role, created_at
FROM project_members
WHERE org_id = @org_id AND project_id = @project_id
ORDER BY created_at;

-- name: RemoveProjectMember :execrows
DELETE FROM project_members
WHERE org_id = @org_id AND project_id = @project_id AND user_id = @user_id;
