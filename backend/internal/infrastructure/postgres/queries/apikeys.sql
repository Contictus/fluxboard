-- API keys — Phase 6 (docs/build/PHASE-6-ADMIN-OBS.md §4, FR-API-001/002).
-- api_keys is [T] (RLS). Create/List/Revoke run under a tenant tx (TenantPool).
-- GetByHash + TouchLastUsed authenticate an inbound key BEFORE any tenant context
-- exists, so their repo methods run on the owner pool (RLS-bypassing); the hash is
-- globally unique + unguessable, making the cross-org lookup safe.

-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, org_id, prefix, key_hash, name, scopes, created_by)
VALUES (@id, @org_id, @prefix, @key_hash, @name, @scopes, @created_by);

-- name: ListAPIKeysByOrg :many
SELECT id, org_id, prefix, key_hash, name, scopes, created_by,
       last_used_at, revoked_at, created_at
FROM api_keys
WHERE org_id = @org_id
ORDER BY created_at DESC;

-- name: GetAPIKeyByHash :one
-- Auth-path lookup (owner pool). Revoked keys ARE returned so the caller maps them
-- to 401 rather than a silent miss.
SELECT id, org_id, prefix, key_hash, name, scopes, created_by,
       last_used_at, revoked_at, created_at
FROM api_keys
WHERE key_hash = @key_hash;

-- name: RevokeAPIKey :execrows
-- rows-affected 0 ⇒ absent or already revoked ⇒ caller returns ErrNotFound.
UPDATE api_keys
SET revoked_at = now()
WHERE org_id = @org_id AND id = @id AND revoked_at IS NULL;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE key_hash = @key_hash;
