-- Notification preferences — Phase 5 (FR-NTF-004). notification_prefs [T]:
-- one row per (org, user, category); absent row ⇒ opt-in default in the domain.

-- name: ListNotificationPrefs :many
SELECT org_id, user_id, category, email, in_app
FROM notification_prefs
WHERE org_id = @org_id AND user_id = @user_id;

-- name: GetNotificationPref :one
SELECT org_id, user_id, category, email, in_app
FROM notification_prefs
WHERE org_id = @org_id AND user_id = @user_id AND category = @category;

-- name: UpsertNotificationPref :exec
INSERT INTO notification_prefs (org_id, user_id, category, email, in_app)
VALUES (@org_id, @user_id, @category, @email, @in_app)
ON CONFLICT (org_id, user_id, category) DO UPDATE SET
  email = EXCLUDED.email, in_app = EXCLUDED.in_app;
