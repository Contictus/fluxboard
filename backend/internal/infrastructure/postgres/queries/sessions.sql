-- name: CreateSession :exec
INSERT INTO sessions (id, family_id, user_id, token_hash, user_agent, ip, expires_at)
VALUES (@id, @family_id, @user_id, @token_hash, @user_agent, nullif(@ip, '')::inet, @expires_at);

-- name: GetSessionByID :one
SELECT id, family_id, user_id, token_hash, user_agent,
       coalesce(host(ip), '')::text AS ip,
       expires_at, rotated_at, revoked_at,
       coalesce(revoke_reason, '')::text AS revoke_reason, created_at
FROM sessions
WHERE id = @id;

-- name: GetSessionByTokenHashForUpdate :one
SELECT id, family_id, user_id, token_hash, user_agent,
       coalesce(host(ip), '')::text AS ip,
       expires_at, rotated_at, revoked_at,
       coalesce(revoke_reason, '')::text AS revoke_reason, created_at
FROM sessions
WHERE token_hash = @token_hash
FOR UPDATE;

-- name: MarkSessionRotated :exec
UPDATE sessions SET rotated_at = now() WHERE id = @id;

-- name: RevokeSessionByID :exec
UPDATE sessions
SET revoked_at = now(), revoke_reason = @reason
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeSessionFamily :exec
UPDATE sessions
SET revoked_at = now(), revoke_reason = @reason
WHERE family_id = @family_id AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE sessions
SET revoked_at = now(), revoke_reason = @reason
WHERE user_id = @user_id AND revoked_at IS NULL;
