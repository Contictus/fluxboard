-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash, name, email_verified, platform_role)
VALUES (@id, @email, nullif(@password_hash, '')::text, @name, @email_verified, @platform_role);

-- name: GetUserByEmail :one
SELECT id, email,
       coalesce(password_hash, '')::text AS password_hash,
       name,
       coalesce(avatar_key, '')::text AS avatar_key,
       email_verified, platform_role,
       coalesce(totp_secret, '')::text AS totp_secret,
       totp_enabled, locked_at, created_at, updated_at
FROM users
WHERE email = @email;

-- name: GetUserByID :one
SELECT id, email,
       coalesce(password_hash, '')::text AS password_hash,
       name,
       coalesce(avatar_key, '')::text AS avatar_key,
       email_verified, platform_role,
       coalesce(totp_secret, '')::text AS totp_secret,
       totp_enabled, locked_at, created_at, updated_at
FROM users
WHERE id = @id;

-- name: UpdateUserPasswordHash :exec
UPDATE users SET password_hash = @password_hash WHERE id = @id;

-- name: MarkUserEmailVerified :exec
UPDATE users SET email_verified = true WHERE id = @id;

-- name: SetUserTOTP :exec
-- Sets (or clears, via empty secret) the encrypted TOTP secret and its enabled
-- flag together (docs/04-AUTH.md §4 TOTP step).
UPDATE users
SET totp_secret = nullif(@totp_secret, '')::text, totp_enabled = @totp_enabled
WHERE id = @id;

-- name: UpdateUserName :exec
UPDATE users SET name = @name WHERE id = @id;

-- name: UpdateUserAvatarKey :exec
-- Sets (or clears, via empty string) the MinIO object key for the user's avatar.
UPDATE users SET avatar_key = nullif(@avatar_key, '')::text WHERE id = @id;

-- name: DeleteUser :exec
-- Hard-deletes the account (docs/08 §3 DELETE /me). Memberships/sessions cascade
-- via ON DELETE CASCADE. Callers MUST enforce the sole-owner guard first.
DELETE FROM users WHERE id = @id;

-- name: ListSoleOwnerOrgs :many
-- Orgs the user solely owns (blocks account deletion, docs/08 §3). Cross-org read:
-- run on the owner pool (memberships RLS is non-FORCE, table owner bypasses it).
SELECT o.id, o.slug, o.name
FROM memberships m
JOIN organizations o ON o.id = m.org_id
WHERE m.user_id = @user_id
  AND m.role = 'OWNER'
  AND o.deleted_at IS NULL
  AND (SELECT count(*) FROM memberships m2
       WHERE m2.org_id = m.org_id AND m2.role = 'OWNER') = 1;
