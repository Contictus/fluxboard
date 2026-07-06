-- Projects ([T], tenant-scoped) ---------------------------------------------

-- name: CreateProject :exec
INSERT INTO projects (id, org_id, key, name, description, color, visibility, created_by)
VALUES (@id, @org_id, @key, @name, @description, @color, @visibility, @created_by);

-- name: GetProject :one
SELECT id, org_id, key, name, description, color, visibility,
       archived_at, created_by, created_at, updated_at
FROM projects
WHERE org_id = @org_id AND id = @id;

-- name: ListProjects :many
-- Visibility filter (FR-PROJ-002/003): see_all (org ADMIN+) returns every
-- project; otherwise 'org'-visible plus 'private' ones the user is a member of.
SELECT p.id, p.org_id, p.key, p.name, p.description, p.color, p.visibility,
       p.archived_at, p.created_by, p.created_at, p.updated_at
FROM projects p
WHERE p.org_id = @org_id
  AND (sqlc.arg('include_archived')::bool OR p.archived_at IS NULL)
  AND (
        sqlc.arg('see_all')::bool
        OR p.visibility = 'org'
        OR EXISTS (
          SELECT 1 FROM project_members pm
          WHERE pm.project_id = p.id AND pm.user_id = @user_id
        )
      )
ORDER BY p.created_at DESC;

-- name: UpdateProject :execrows
UPDATE projects
SET name = @name, description = @description, color = @color, visibility = @visibility
WHERE org_id = @org_id AND id = @id;

-- name: SetProjectArchived :execrows
UPDATE projects
SET archived_at = sqlc.narg('archived_at')
WHERE org_id = @org_id AND id = @id;

-- name: NextTaskNumber :one
-- Atomically bump and return the per-project task counter (FR-TASK-001). The
-- row lock serializes concurrent creates; gaps are acceptable.
UPDATE projects SET task_counter = task_counter + 1
WHERE org_id = @org_id AND id = @id
RETURNING task_counter;

-- name: CountProjectsByOrg :one
-- Live (non-archived) project count for the plan-limit gate (FR-BILL-009).
SELECT count(*) FROM projects WHERE org_id = @org_id AND archived_at IS NULL;
