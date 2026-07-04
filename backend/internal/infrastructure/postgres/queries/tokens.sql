-- name: CreateOneTimeToken :exec
INSERT INTO one_time_tokens (id, purpose, user_id, token_hash, payload, expires_at)
VALUES (@id, @purpose, @user_id, @token_hash, @payload, @expires_at);

-- name: ConsumeOneTimeToken :one
UPDATE one_time_tokens
SET used_at = now()
WHERE purpose = @purpose
  AND token_hash = @token_hash
  AND used_at IS NULL
  AND expires_at > now()
RETURNING id, purpose, user_id, token_hash, payload, expires_at, used_at, created_at;
