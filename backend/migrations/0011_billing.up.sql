-- 0011_billing — Phase 4 (docs/06-BILLING.md, docs/07 §Billing, FR-BILL-001..009).
-- Six tables backing Stripe subscriptions, the webhook idempotency ledger, usage
-- metering, and the transactional outbox. DDL only; RLS lands in 0012 (the
-- DDL/RLS split, as 0007/0008 and 0009/0010).
--
-- Global (NO RLS, no org_id): plans, processed_stripe_events. plans is reference
-- data; processed_stripe_events is the webhook dedup ledger written from the
-- unauthenticated webhook endpoint (no tenant context) — see 0012.
--
-- Plan pointer lives on subscriptions.plan_code (one subscription row per org),
-- NOT on organizations: an org with no row resolves to Free (status 'none').
-- organizations.stripe_customer_id already exists since 0003 (wired here, not
-- re-added). Money = integer minor units (invariant #5). Times = timestamptz UTC.

-- plans — reference data (06 §1). Seeded by cmd/stripeseed; in MODE=stub the seed
-- writes static rows and leaves the *_price_id columns null (no live Stripe call).
CREATE TABLE plans (
  code                     text PRIMARY KEY,          -- 'free' | 'pro' | 'business'
  name                     text NOT NULL,
  stripe_product_id        text,
  seat_price_id            text,
  metered_storage_price_id text,
  metered_api_price_id     text,
  max_members              int    NOT NULL,           -- -1 = unlimited
  max_projects             int    NOT NULL,           -- -1 = unlimited
  max_storage_bytes        bigint NOT NULL,
  api_rate_per_min         int    NOT NULL,
  audit_retention_days     int    NOT NULL,
  metered                  boolean NOT NULL DEFAULT false
);

-- subscriptions [T] — local mirror of the Stripe subscription (06 §2). One per
-- org (org_id UNIQUE). Stripe is the source of truth; this row is reconciled by
-- the webhook consumer and the nightly reconcile job. last_stripe_event_at is the
-- staleness guard for out-of-order webhook delivery (06 §4).
CREATE TABLE subscriptions (                          -- [T]
  id                     uuid PRIMARY KEY,
  org_id                 uuid NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
  plan_code              text NOT NULL DEFAULT 'free' REFERENCES plans(code),
  stripe_subscription_id text UNIQUE,
  stripe_customer_id     text,
  status                 text NOT NULL DEFAULT 'none'
        CHECK (status IN ('none','trialing','active','past_due','unpaid','canceled')),
  current_period_end     timestamptz,
  cancel_at_period_end   boolean NOT NULL DEFAULT false,
  last_stripe_event_at   timestamptz,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER subscriptions_set_updated_at BEFORE UPDATE ON subscriptions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- invoices [T] — mirror synced on invoice.finalized / invoice.payment_succeeded
-- (06 §4). Read-only history page (FR-BILL-008) links to the Stripe-hosted PDF.
CREATE TABLE invoices (                               -- [T]
  id                uuid PRIMARY KEY,
  org_id            uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  stripe_invoice_id text NOT NULL UNIQUE,
  number            text,
  status            text NOT NULL,                     -- draft|open|paid|void|uncollectible
  amount_due        bigint NOT NULL DEFAULT 0,         -- minor units
  amount_paid       bigint NOT NULL DEFAULT 0,
  currency          text   NOT NULL DEFAULT 'usd',
  hosted_pdf_url    text,
  period_start      timestamptz,
  period_end        timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX invoices_org_idx ON invoices (org_id, created_at DESC);

-- processed_stripe_events — GLOBAL idempotency ledger (06 §4). The webhook
-- endpoint runs without tenant context, so this has no org_id and no RLS. The
-- INSERT ... ON CONFLICT (event_id) DO NOTHING inside the side-effect tx is the
-- dedup primitive: rows-affected 0 ⇒ duplicate ⇒ skip side effects, respond 200.
CREATE TABLE processed_stripe_events (
  event_id     text PRIMARY KEY,
  type         text NOT NULL,
  payload      jsonb NOT NULL,
  handled      boolean NOT NULL DEFAULT true,          -- false = recognized but no handler (admin browser)
  error        text,
  processed_at timestamptz NOT NULL DEFAULT now()
);

-- usage_records [T] — hourly aggregate per (org, metric, day); idempotent UPSERT
-- (06 §5). Daily job pushes Business-plan aggregates to Stripe with action=set.
CREATE TABLE usage_records (                           -- [T]
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  metric      text NOT NULL,                           -- active_members | storage_bytes | api_calls
  period_date date NOT NULL,
  value       bigint NOT NULL DEFAULT 0,
  pushed_at   timestamptz,                             -- last push-to-Stripe time (metered plans)
  PRIMARY KEY (org_id, metric, period_date)
);

-- outbox [T] — transactional outbox (09 §2). Producers INSERT within the same tx
-- as their DB write; the worker's outbox:drain claims undrained rows and hands
-- off to Asynq (TaskID = outbox id ⇒ at-most-once enqueue). Partial index keeps
-- the drain scan tight.
CREATE TABLE outbox (                                  -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  kind       text NOT NULL,                            -- email:send | ...
  payload    jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  drained_at timestamptz
);
CREATE INDEX outbox_undrained_idx ON outbox (created_at) WHERE drained_at IS NULL;
