-- Directory lookups — Phase 5 notification fan-out (notifyuc.Directory). Resolves
-- project members (for @mention matching) and specific users (targeted notifs).
-- Runs inside a tenant tx: project_members is RLS-scoped; users is global.

-- name: ListProjectMemberUsers :many
SELECT u.id, u.email, u.name
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE pm.org_id = @org_id AND pm.project_id = @project_id;

-- name: ListUsersByIDs :many
SELECT id, email, name
FROM users
WHERE id = ANY(@ids::uuid[]);
