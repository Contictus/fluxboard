-- Attachments ([T], tenant-scoped) — FR-TASK-006 -----------------------------

-- name: CreateAttachment :exec
-- Insert the 'pending' row alongside minting a presigned PUT URL.
INSERT INTO attachments (id, org_id, task_id, uploader_id, object_key, filename,
                         content_type, size_bytes, status)
VALUES (@id, @org_id, @task_id, @uploader_id, @object_key, @filename,
        @content_type, @size_bytes, 'pending');

-- name: GetAttachment :one
SELECT id, org_id, task_id, uploader_id, object_key, filename, content_type,
       size_bytes, status, created_at, confirmed_at
FROM attachments
WHERE org_id = @org_id AND id = @id;

-- name: CommitAttachment :execrows
-- Flip a pending row to committed (idempotency: no-op if already committed via
-- the WHERE status filter). size_bytes is corrected to the HEAD-verified size.
UPDATE attachments
SET status = 'committed', size_bytes = @size_bytes, confirmed_at = @confirmed_at
WHERE org_id = @org_id AND id = @id AND status = 'pending';

-- name: ListAttachmentsByTask :many
SELECT id, org_id, task_id, uploader_id, object_key, filename, content_type,
       size_bytes, status, created_at, confirmed_at
FROM attachments
WHERE org_id = @org_id AND task_id = @task_id AND status = 'committed'
ORDER BY created_at;

-- name: DeleteAttachment :execrows
DELETE FROM attachments WHERE org_id = @org_id AND id = @id;

-- name: SumOrgAttachmentBytes :one
-- Total committed bytes for the org (storage-quota check, FR-TASK-006).
SELECT COALESCE(SUM(size_bytes), 0)::bigint FROM attachments
WHERE org_id = @org_id AND status = 'committed';

-- name: ListOrphanAttachments :many
-- Pending rows older than the cutoff (never PUT or never confirmed) — the nightly
-- orphan GC removes their objects then their rows (per-tenant).
SELECT id, org_id, task_id, uploader_id, object_key, filename, content_type,
       size_bytes, status, created_at, confirmed_at
FROM attachments
WHERE org_id = @org_id AND status = 'pending' AND created_at < @cutoff;
