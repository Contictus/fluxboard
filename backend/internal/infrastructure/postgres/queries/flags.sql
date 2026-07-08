-- Feature flags — Phase 6 (FR-ADM-006). feature_flags is [T] (RLS via TenantPool).

-- name: ListFeatureFlags :many
SELECT org_id, flag, enabled FROM feature_flags
WHERE org_id = @org_id
ORDER BY flag;

-- name: UpsertFeatureFlag :exec
INSERT INTO feature_flags (org_id, flag, enabled)
VALUES (@org_id, @flag, @enabled)
ON CONFLICT (org_id, flag) DO UPDATE SET enabled = EXCLUDED.enabled;
