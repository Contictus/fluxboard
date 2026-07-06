-- Billing — Phase 4 (docs/06-BILLING.md, FR-BILL-001..009) ---------------------
-- plans + processed_stripe_events are GLOBAL (no RLS); subscriptions + invoices
-- are [T] (RLS via TenantPool). The webhook consumer writes the global ledger and
-- the [T] mirrors in one transaction (webhook_repo.go).

-- name: GetPlan :one
SELECT code, name, stripe_product_id, seat_price_id, metered_storage_price_id,
       metered_api_price_id, max_members, max_projects, max_storage_bytes,
       api_rate_per_min, audit_retention_days, metered
FROM plans
WHERE code = @code;

-- name: ListPlans :many
SELECT code, name, stripe_product_id, seat_price_id, metered_storage_price_id,
       metered_api_price_id, max_members, max_projects, max_storage_bytes,
       api_rate_per_min, audit_retention_days, metered
FROM plans
ORDER BY max_projects; -- free, pro, business by ascending caps (business = -1 sorts first, acceptable)

-- name: GetSubscription :one
SELECT id, org_id, plan_code, stripe_subscription_id, stripe_customer_id, status,
       current_period_end, cancel_at_period_end, last_stripe_event_at,
       created_at, updated_at
FROM subscriptions
WHERE org_id = @org_id;

-- name: GetSubscriptionLastEvent :one
-- Staleness read for the out-of-order webhook guard (06 §4). ErrNoRows = no row.
SELECT last_stripe_event_at
FROM subscriptions
WHERE org_id = @org_id;

-- name: UpsertSubscription :exec
-- Full desired state, keyed on org_id. The caller owns last_stripe_event_at:
-- persistCustomer passes the row's existing value; the webhook passes the event
-- time (after its staleness guard), so EXCLUDED never regresses it.
INSERT INTO subscriptions (id, org_id, plan_code, stripe_subscription_id,
                           stripe_customer_id, status, current_period_end,
                           cancel_at_period_end, last_stripe_event_at)
VALUES (@id, @org_id, @plan_code, @stripe_subscription_id, @stripe_customer_id,
        @status, @current_period_end, @cancel_at_period_end, @last_stripe_event_at)
ON CONFLICT (org_id) DO UPDATE SET
  plan_code              = EXCLUDED.plan_code,
  stripe_subscription_id = EXCLUDED.stripe_subscription_id,
  stripe_customer_id     = EXCLUDED.stripe_customer_id,
  status                 = EXCLUDED.status,
  current_period_end     = EXCLUDED.current_period_end,
  cancel_at_period_end   = EXCLUDED.cancel_at_period_end,
  last_stripe_event_at   = EXCLUDED.last_stripe_event_at;

-- name: UpsertInvoice :exec
INSERT INTO invoices (id, org_id, stripe_invoice_id, number, status, amount_due,
                      amount_paid, currency, hosted_pdf_url, period_start, period_end)
VALUES (@id, @org_id, @stripe_invoice_id, @number, @status, @amount_due,
        @amount_paid, @currency, @hosted_pdf_url, @period_start, @period_end)
ON CONFLICT (stripe_invoice_id) DO UPDATE SET
  number         = EXCLUDED.number,
  status         = EXCLUDED.status,
  amount_due     = EXCLUDED.amount_due,
  amount_paid    = EXCLUDED.amount_paid,
  currency       = EXCLUDED.currency,
  hosted_pdf_url = EXCLUDED.hosted_pdf_url,
  period_start   = EXCLUDED.period_start,
  period_end     = EXCLUDED.period_end;

-- name: ListInvoicesByOrg :many
SELECT id, org_id, stripe_invoice_id, number, status, amount_due, amount_paid,
       currency, hosted_pdf_url, period_start, period_end, created_at
FROM invoices
WHERE org_id = @org_id
ORDER BY created_at DESC;

-- name: InsertProcessedEvent :execrows
-- Webhook dedup primitive (06 §4): rows-affected 0 ⇒ duplicate. Runs both inside
-- the tenant tx (webhook_repo) and on the plain pool (processed_event_repo).
INSERT INTO processed_stripe_events (event_id, type, payload, handled, error)
VALUES (@event_id, @type, @payload, @handled, @error)
ON CONFLICT (event_id) DO NOTHING;
