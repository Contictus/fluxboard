-- Transactional outbox — Phase 4 / 09 §2 --------------------------------------
-- outbox [T]: producers INSERT within their write tx (the webhook does so in
-- webhook_repo); the worker's outbox:drain claims undrained rows and enqueues to
-- Asynq with TaskID = outbox id (at-most-once enqueue).

-- name: InsertOutbox :exec
INSERT INTO outbox (id, org_id, kind, payload)
VALUES (@id, @org_id, @kind, @payload);

-- name: ClaimOutboxBatch :many
-- Atomically mark up to @lim undrained rows drained and return them. FOR UPDATE
-- SKIP LOCKED lets concurrent drainers make progress without contending.
UPDATE outbox SET drained_at = now()
WHERE id IN (
  SELECT o.id FROM outbox o
  WHERE o.org_id = @org_id AND o.drained_at IS NULL
  ORDER BY o.created_at
  LIMIT @lim
  FOR UPDATE SKIP LOCKED
)
RETURNING id, org_id, kind, payload, created_at, drained_at;
