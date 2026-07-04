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
