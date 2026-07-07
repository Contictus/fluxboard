-- Notifications — Phase 5 (docs/09-REALTIME-JOBS.md §3, FR-NTF-002). notifications
-- [T]: cursor pagination by created_at DESC; unread = read_at IS NULL.

-- name: InsertNotification :exec
INSERT INTO notifications (id, org_id, user_id, category, title, body, entity_type, entity_id, created_at)
VALUES (@id, @org_id, @user_id, @category, @title, @body, @entity_type, @entity_id, @created_at);

-- name: ListNotifications :many
-- only_unread=false returns all; before nil ⇒ newest page.
SELECT id, org_id, user_id, category, title, body, entity_type, entity_id, read_at, created_at
FROM notifications
WHERE org_id = @org_id AND user_id = @user_id
  AND (@only_unread::bool = false OR read_at IS NULL)
  AND (sqlc.narg('before')::timestamptz IS NULL OR created_at < sqlc.narg('before'))
ORDER BY created_at DESC
LIMIT @lim;

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications
WHERE org_id = @org_id AND user_id = @user_id AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = @read_at
WHERE org_id = @org_id AND user_id = @user_id AND id = @id AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications SET read_at = @read_at
WHERE org_id = @org_id AND user_id = @user_id AND read_at IS NULL;
