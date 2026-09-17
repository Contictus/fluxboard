-- Time entries ([T], tenant-scoped, FR-TIME) -------------------------------

-- name: CreateTimeEntry :exec
INSERT INTO time_entries (id, org_id, task_id, user_id, started_at, ended_at, note)
VALUES (@id, @org_id, @task_id, @user_id, @started_at, @ended_at, @note);

-- name: GetTimeEntry :one
SELECT id, org_id, task_id, user_id, started_at, ended_at, note, created_at
FROM time_entries
WHERE org_id = @org_id AND id = @id;

-- name: ListTimeEntriesByTask :many
SELECT id, org_id, task_id, user_id, started_at, ended_at, note, created_at
FROM time_entries
WHERE org_id = @org_id AND task_id = @task_id
ORDER BY started_at DESC;

-- name: ListRunningTimeEntries :many
SELECT id, org_id, task_id, user_id, started_at, ended_at, note, created_at
FROM time_entries
WHERE org_id = @org_id AND user_id = @user_id AND ended_at IS NULL
ORDER BY started_at DESC;

-- name: StopTimeEntry :execrows
UPDATE time_entries SET ended_at = @ended_at
WHERE org_id = @org_id AND id = @id AND ended_at IS NULL;

-- name: DeleteTimeEntry :execrows
DELETE FROM time_entries
WHERE org_id = @org_id AND id = @id;
