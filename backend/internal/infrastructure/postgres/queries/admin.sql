-- Platform admin cross-tenant reads — Phase 6 (FR-ADM-002/004). These span ALL
-- orgs, so the repo runs them on the OWNER pool (table owner bypasses the ENABLE
-- (non-FORCE) RLS on subscriptions/memberships). Gated by the platform-admin HTTP
-- guard. MRR = the org's plan monthly_price when its subscription is entitled.

-- name: ListTenants :many
SELECT o.id,
       o.name,
       o.slug::text                        AS slug,
       COALESCE(s.plan_code, 'free')        AS plan_code,
       COALESCE(s.status, 'none')           AS status,
       (SELECT COUNT(*) FROM memberships m WHERE m.org_id = o.id) AS member_count,
       CASE WHEN COALESCE(s.status, 'none') IN ('active', 'trialing')
            THEN COALESCE(p.monthly_price, 0) ELSE 0 END::bigint  AS mrr,
       o.created_at
FROM organizations o
LEFT JOIN subscriptions s ON s.org_id = o.id
LEFT JOIN plans p          ON p.code   = s.plan_code
WHERE o.deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR o.name ILIKE '%' || sqlc.narg('search') || '%'
       OR o.slug::text ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.narg('plan')::text   IS NULL OR COALESCE(s.plan_code, 'free') = sqlc.narg('plan'))
  AND (sqlc.narg('status')::text IS NULL OR COALESCE(s.status, 'none')     = sqlc.narg('status'))
ORDER BY o.created_at DESC
LIMIT @lim OFFSET @off;

-- name: GetTenantSummary :one
SELECT o.id,
       o.name,
       o.slug::text                        AS slug,
       COALESCE(s.plan_code, 'free')        AS plan_code,
       COALESCE(s.status, 'none')           AS status,
       (SELECT COUNT(*) FROM memberships m WHERE m.org_id = o.id) AS member_count,
       CASE WHEN COALESCE(s.status, 'none') IN ('active', 'trialing')
            THEN COALESCE(p.monthly_price, 0) ELSE 0 END::bigint  AS mrr,
       o.created_at
FROM organizations o
LEFT JOIN subscriptions s ON s.org_id = o.id
LEFT JOIN plans p          ON p.code   = s.plan_code
WHERE o.id = @org_id AND o.deleted_at IS NULL;

-- name: WebhookEventsForCustomer :many
-- The global ledger is not org-tagged; link to an org via the Stripe customer id
-- embedded in the event payload (present on subscription/invoice events). Events
-- without a customer simply do not match.
SELECT event_id, type, handled, COALESCE(error, '') AS error, processed_at
FROM processed_stripe_events
WHERE payload #>> '{data,object,customer}' = @customer_id::text
ORDER BY processed_at DESC
LIMIT @lim;

-- name: GetProcessedEventPayload :one
-- Backs webhook:retry — the worker replays this stored payload through the consumer.
SELECT event_id, type, payload FROM processed_stripe_events WHERE event_id = @event_id;
