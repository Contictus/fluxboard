-- Entitlement overrides — Phase 6 (FR-ADM-002). entitlement_overrides is [T]
-- (RLS via TenantPool).

-- name: ListOverrides :many
SELECT org_id, key, value, note, created_by, created_at FROM entitlement_overrides
WHERE org_id = @org_id
ORDER BY key;

-- name: UpsertOverride :exec
INSERT INTO entitlement_overrides (org_id, key, value, note, created_by, created_at)
VALUES (@org_id, @key, @value, @note, @created_by, @created_at)
ON CONFLICT (org_id, key) DO UPDATE SET
  value      = EXCLUDED.value,
  note       = EXCLUDED.note,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at;

-- name: DeleteOverride :execrows
DELETE FROM entitlement_overrides WHERE org_id = @org_id AND key = @key;
