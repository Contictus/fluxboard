-- name: DeleteUserRecoveryCodes :exec
-- Clears any prior codes before a fresh batch is issued (activate / regenerate).
DELETE FROM recovery_codes WHERE user_id = @user_id;

-- name: CreateRecoveryCode :exec
INSERT INTO recovery_codes (user_id, code_hash) VALUES (@user_id, @code_hash);

-- name: ConsumeRecoveryCode :execrows
-- Single-use consume: marks a matching unused code used. Rows affected = 1 means
-- the code was valid and is now spent; 0 means invalid/already-used.
UPDATE recovery_codes
SET used_at = now()
WHERE user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;
