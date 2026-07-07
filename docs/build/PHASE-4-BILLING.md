# Phase 4 — Billing (Stripe)

**Modules:** BILL-001..011 · **Spec:** `docs/06-BILLING.md`, `docs/01-…md` §BILL.
**Depends on:** Phase 3 (done). **Gate:** checkout → webhook → entitlement e2e green.

> **[DECISION 4.0.1] Stripe integration mode — resolve before section 1.**
> (a) **Real test keys** — put `STRIPE_SECRET_KEY` (test) + webhook secret in
> `.env`, run the `stripe` compose profile; SDK hits Stripe's test API. Most
> faithful; needed for `stripe trigger` e2e.
> (b) **Stub port** — implement `billing.StripeGateway` with a dev fake; wire real
> `stripe-go` behind it later. No keys now; webhook e2e uses crafted payloads.
> Record the choice here when made: **`MODE = stub`** (resolved 2026-07-06).
> Dev-fake `StripeGateway`; no keys; real `stripe-go/v78` wired behind the same
> interface later. Webhook e2e uses crafted signed payloads (stub `ConstructEvent`
> accepts an `X-Stub-Signature` shared secret). `stripe trigger` / compose
> `stripe` profile skipped (4.0.5 N/A).

Invariants in play: money = integer minor units; webhook idempotency
(processed_stripe_events UNIQUE, same tx as side effects); Stripe is source of
truth for subscription state; card data never touches our servers (Checkout +
Portal only).

---

## Section 0 — Prereqs & decisions

- [x] 4.0.1 [DECISION] Resolve Stripe mode (banner above); record `MODE`. → **MODE=stub** (banner).
- [~] 4.0.2 `stripe-go/v78` — **deferred to §4.4** (stub gateway needs no external SDK; real adapter pulls the dep). Keeps `go.mod` clean under MODE=stub.
- [x] 4.0.3 [DECISION] Freeze plan tiers + limits table (Free/Pro/Business). **Frozen** (table below).
- [x] 4.0.4 Billing env in `config.go` — added `STRIPE_MODE`(default stub) + `STRIPE_PRICE_SEED`; `STRIPE_SECRET_KEY`/`STRIPE_WEBHOOK_SECRET` already present + masked in `String()`.
- [x] 4.0.5 [VERIFY] ~~stripe-cli profile~~ — **N/A (MODE=stub)**.

> **Frozen plan tiers (4.0.3)** — money-independent limits; `-1` = unlimited.
> Seeded into `plans` by 4.1.11 (static rows, no live Stripe call in stub mode).
>
> | code | max_members | max_projects | max_storage_bytes | api_rate_per_min | audit_retention_days | metered |
> |---|---|---|---|---|---|---|
> | free | 5 | 3 | 2147483648 (2 GiB) | 60 | 7 | false |
> | pro | 25 | 50 | 53687091200 (50 GiB) | 300 | 30 | false |
> | business | -1 | -1 | 536870912000 (500 GiB) | 1200 | 365 | true |
>
> Free storage = 2 GiB matches existing app const `OrgStorageQuotaBytes` (2<<30);
> 4.6.2 aligns the attachment quota to plan `max_storage_bytes`.

## Section 1 — Migrations (0011 DDL, 0012 RLS)

- [x] 4.1.1 `0011_billing.up.sql`: `plans` (code PK, name, stripe_product_id, seat_price_id, metered_storage_price_id, metered_api_price_id, max_members, max_projects, max_storage_bytes, api_rate_per_min, audit_retention_days, metered bool). FR-BILL-002.
- [x] 4.1.2 `0011`: `subscriptions` [T] (id PK, org_id UNIQUE, plan_code FK, stripe_subscription_id UNIQUE, stripe_customer_id, status CHECK, current_period_end, cancel_at_period_end, last_stripe_event_at, created/updated_at + set_updated_at trigger). FR-BILL-005, 06 §2.
- [x] 4.1.3 `0011`: `invoices` [T] mirror (id, org_id, stripe_invoice_id UNIQUE, number, status, amount_due, amount_paid, currency, hosted_pdf_url, period_start/end, created_at) + `invoices_org_idx`. FR-BILL-008.
- [x] 4.1.4 `0011`: `processed_stripe_events` (event_id PK, type, payload jsonb, handled bool, error text, processed_at). Global (no org_id) — webhook dedup core. FR-BILL-005.
- [x] 4.1.5 `0011`: `usage_records` [T] (org_id, metric, period_date, value bigint, pushed_at, `PK(org_id,metric,period_date)`). FR-BILL-007.
- [x] 4.1.6 `0011`: `outbox` [T] (id, org_id, kind, payload jsonb, created_at, drained_at) + partial index `outbox_undrained_idx WHERE drained_at IS NULL`. 09 §2.
- [x] 4.1.7 `0011`: plan pointer modeled on `subscriptions.plan_code` (org with no row ⇒ Free); `organizations.stripe_customer_id` already exists since 0003 (no DDL) — documented in the 0011 header. No org-table change.
- [x] 4.1.8 `0011.down.sql`: reverse-order drops.
- [x] 4.1.9 `0012_billing_rls.up/.down.sql`: ENABLE RLS + `tenant_isolation` on `subscriptions`, `invoices`, `usage_records`, `outbox`; `plans` + `processed_stripe_events` stay global.
- [x] 4.1.10 [VERIFY] `migrate up` → `down 2` → `up` clean on 0011/0012 (live docker). App grants auto-applied via `ALTER DEFAULT PRIVILEGES` (init 01-roles.sql) — confirmed app role has I/S/U on `subscriptions`.
- [x] 4.1.11 `cmd/stripeseed/main.go`: idempotent `ON CONFLICT (code) DO UPDATE` upsert of the 3 static plan rows (MODE=stub; live branch errors as not-yet-impl). Ran → 3 rows verified. FR-BILL-002.

## Section 2 — Domain (`internal/domain/billing/`)

- [x] 4.2.1 `billing.go`: `Plan` model + `PlanCode` enum (free/pro/business) with limits; `Entitlements` struct (06 §7).
- [x] 4.2.2 `billing.go`: `Subscription` model + `SubStatus` enum (active/trialing/past_due/unpaid/canceled) + `CanTransition`/`DeriveEntitlements(plan,status)` per state machine 06 §2.
- [x] 4.2.3 `billing.go`: `Invoice`, `UsageRecord`, `OutboxItem`, `ProcessedEvent` models. Money fields int64 minor units.
- [x] 4.2.4 `ports.go`: repo ports — `SubscriptionRepository`, `InvoiceRepository`, `ProcessedEventRepository`, `UsageRepository`, `OutboxRepository` + `PlanRepository`/`WebhookRepository` (methods orgID-first for [T]).
- [x] 4.2.5 `ports.go`: `StripeGateway` (CreateCheckout, UpcomingInvoice, UpdateSubscription, CancelSubscription, PortalSession, PushUsage, ConstructEvent) + `EntitlementCache` (Get/Set/Bust).
- [x] 4.2.6 [VERIFY] `project_test.go`-style unit test: every state-machine edge + `DeriveEntitlements` for each (plan,status) pair. FR-BILL-004/006. → `go test ./internal/domain/billing/` green (5 tests).

## Section 3 — Usecase (`internal/usecase/billinguc/`)

- [x] 4.3.1 `service.go`: `Summary(orgID)` → plan, renewal, cancel flag, status/warning + resolved entitlement limits (`SummaryView`); `Resolve(orgID)` cache-first entitlements shared with §5 middleware. Usage-vs-limits composed at HTTP layer (counts live in project/tenant domains, not billing — layering). `ListInvoices` for §5.4. FR-BILL-001/009.
- [x] 4.3.2 `Checkout(orgID, planCode, seats)` → lazy Stripe customer (EnsureCustomer + persist on sub row), `CreateCheckout` with idempotency key `co:{org}:{plan}:{unix/300}`, success/cancel URLs from `BaseURL`; rejects `free`. FR-BILL-002, 06 §3.
- [x] 4.3.3 `PreviewChange(orgID, planCode)` → `UpcomingInvoice` proration figure (requires live Stripe sub, else ErrConflict). FR-BILL-003. TODO(seats): qty not mirrored, previews 1.
- [x] 4.3.4 `ApplyChange(orgID, planCode)` → `UpdateSubscription(create_prorations)` under idem key `chg:{org}:{plan}:{unix/300}`; busts cache. FR-BILL-003.
- [x] 4.3.5 `Cancel(orgID, atPeriodEnd)` / `Resume(orgID)` → gateway + cache bust. Immediate cancel = admin-only (Phase 6). FR-BILL-004.
- [x] 4.3.6 `Portal(orgID)` → Billing Portal session URL. FR-BILL-006.
- [x] 4.3.7 `webhook.go` `ProcessEvent(event)`: unroutable→global ledger handled=false; routable→build `WebhookMutation` (ledger + side effects) → `WebhookRepository.Apply` (dedup+staleness+effects in ONE tx, infra §4.4) → bust cache on `result.SubscriptionCh`. FR-BILL-005, 06 §4.
- [x] 4.3.8 Event handlers (`buildSideEffects`): `checkout.session.completed`(+welcome), `customer.subscription.updated`, `customer.subscription.deleted`(+OWNER email), `invoice.payment_succeeded`(recovery→active+resolved), `invoice.payment_failed`(→past_due+dunning), `invoice.finalized`, else→`handled=false`. Sub events build full mirror; invoice events merge status onto current row. 06 §4 table. Unit tests (10) green.

## Section 4 — Infra

- [x] 4.4.1 `internal/infrastructure/stripe/client.go` (pkg `stripex`): `New(mode,secret,baseURL)` → `StubGateway` (MODE=stub) impl of `billing.StripeGateway`; `live` errors not-impl (§4.0.2). (fake impl, MODE=stub.)
- [x] 4.4.2 `stripex`: CreateCheckout (fake URL w/ idem key), EnsureCustomer (deterministic `cus_stub_{org}`), UpdateSubscription/CancelSubscription/Resume (no-op), UpcomingInvoice (stub proration), PortalSession (fake URL).
- [x] 4.4.3 `stripex`: PushUsage — no-op stub (real `UsageRecord.New` Action="set" in live adapter). FR-BILL-007.
- [x] 4.4.4 `stripex`: ConstructEvent — HMAC-SHA256 shared-secret verify of raw body (fail-closed on empty secret/sig → ErrValidation) + crafted-payload→`billing.StripeEvent` mapper; `Sign()` helper for tests/e2e. (real `webhook.ConstructEventWithOptions` 5-min tolerance in live.) FR-BILL-005. Unit tests (5) green.
- [x] 4.4.5 `postgres/subscription_repo.go` over TenantPool (Get, Upsert full desired state ON CONFLICT(org_id)); `plan_repo.go` (PlanRepo, plain pool — plans global).
- [x] 4.4.6 `postgres/invoice_repo.go` (InvoiceRepo [T] upsert-by-stripe_invoice_id + ListByOrg; ProcessedEventRepo plain pool — `InsertProcessedEvent` ON CONFLICT DO NOTHING, rowsAffected⇒inserted), `usage_repo.go` (UpsertUsage on conflict, ListForPush, MarkPushed).
- [x] 4.4.7 `postgres/outbox_repo.go`: `insertOutbox(tx)` helper within a caller tx + `ClaimBatch` (`UPDATE … WHERE drained_at IS NULL RETURNING`, FOR UPDATE SKIP LOCKED). 09 §2.
- [x] 4.4.8 `internal/infrastructure/redis/entitlement_cache.go`: `ent:{orgID}` JSON, TTL from usecase (60s), write-through `Bust`, corrupt-entry⇒miss. 06 §7.
- [x] 4.4.9 `postgres/webhook_repo.go` (WebhookRepo.Apply — dedup + staleness guard + sub/invoice/outbox side effects in ONE tenant tx). Queries in `queries/{billing,usage,outbox}.sql`; `sqlc generate` clean (added `date→time.Time` override). `go test ./...` 96 pass.

## Section 5 — HTTP

- [x] 4.5.1 `handlers/webhooks.go` (`WebhookHandlers.Stripe`): reads raw body (MaxBytesReader 1 MiB — signature is over exact bytes, so no decode before verify), takes `X-Stub-Signature` (falls back to `Stripe-Signature`), `gateway.ConstructEvent` → `svc.ProcessEvent`. 200 on commit/dup/stale, 400 bad-sig/parse (never retried), 500 processing (Stripe retries). Mounted OUTSIDE session auth. FR-BILL-005.
- [x] 4.5.2 `handlers/billing.go` `Summary` → `summaryResp` (plan/status/period-end/cancel flag/past-due/has-sub + nested `entitlements`). FR-BILL-001.
- [x] 4.5.3 `handlers/billing.go`: `Checkout`(→{url}), `PreviewChange`(→{amount,currency}), `ApplyChange`(204), `Cancel`({at_period_end}→204), `Resume`(204), `Portal`(→{url}). FR-BILL-002/003/004/006.
- [x] 4.5.4 `handlers/billing.go` `ListInvoices` → `{invoices:[…]}` newest-first. FR-BILL-008.
- [x] 4.5.5 `router.go`: `/orgs/{orgId}/billing/*` mounted under `read`/`write(ObjBilling)` (ADMIN-only — added `ADMIN billing read` Casbin policy so summary/invoices reads gate too; enforcer_test pins it + MEMBER-denied); `POST /api/v1/webhooks/stripe` at api root, no auth. Deps gains `Billing`/`Webhooks`/`Entitlement` (populated in §6).
- [x] 4.5.6 `middleware/entitlement.go`: `EntitlementGuard.Require(LimitProbe)` → cache-first `Resolve` → probe current-vs-max → `response.PlanLimit` `402 {code:plan_limit_exceeded, details:{limit,current,max}}` (unlimited = max<0 passes). `EntitlementResolver` port declared in middleware pkg (no usecase import). Concrete probes wired per route in §6.2. FR-BILL-009, 06 §7.

## Section 6 — Wiring

- [x] 4.6.1 `cmd/api/main.go`: `stripex.New(cfg.StripeMode, cfg.StripeWebhookSecret, cfg.WebOrigin)` gateway; billing repos (PlanRepo+ProcessedEventRepo on plain pool; Sub/Invoice/Usage/Webhook over tenantPool); `redisx.NewEntitlementCache`; `billinguc.New` (BaseURL=cfg.WebOrigin); `mw.EntitlementGuard{Resolver: billingSvc}`. Set `httpx.Deps` Billing/Webhooks/Entitlement → billing endpoints now live.
- [x] 4.6.2 Entitlement checks on writes: project-create → `EntitlementGuard.RequireProjects()` (max_projects), invitation-create → `RequireMembers()` (max_members) — both middleware gates on the route (rich 402 `{limit,current,max}` via `response.PlanLimit`); counts via new concrete `ProjectRepo.CountByOrg`/`MembershipRepo.CountMembers` (sqlc `CountProjectsByOrg`/`CountMembers`) injected as closures (not on the domain ports → no fake churn). Attachment quota moved into `taskuc.RequestUpload`: optional `EntitlementResolver` → plan `max_storage_bytes` (nil ⇒ Free const), quota exceed now `ErrPlanLimit` (402, was ErrConflict). FR-BILL-004/009.
- [x] 4.6.3 `cmd/worker/main.go` billing deps — landed with §7 (billing repos over tenantPool, stub gateway, entitlement cache + usage counters, mailer, asynq client, drift counter on the metrics registry, queues critical:6/default:3/low:1).

## Section 7 — Jobs (`internal/interface/jobs/`)

- [x] 4.7.1 `outbox:drain` — scheduler `@every 5s` (queue critical); per-org `ClaimBatch(100)` → enqueue per row with `TaskID = outbox:{id}` (`ErrTaskIDConflict` ⇒ dedup success); companion `email:send` handler (queue default, MaxRetry 5) resolves org OWNER emails and sends via `mailer.SendBilling` (welcome/canceled/dunning/resolved templates). 09 §2, ADR-011.
- [x] 4.7.2 Usage counters (Redis, `redisx.UsageCounter`) in request path via new `mw.RateLimiter` on the org router group (api_calls INCR + active_members PFADD, 48h TTL) + `usage:aggregate` hourly UPSERT. **Deviation from 06 §5:** `storage_bytes` is computed at aggregate time from `SUM(size_bytes)` in Postgres (existing `SumOrgAttachmentBytes`) instead of a Redis gauge — simpler, drift-free by construction. **Scope add (user-approved):** the same middleware also ENFORCES plan `api_rate_per_min` (fixed-window per org-minute → `429 rate_limited` + Retry-After, fail-open on Redis trouble), closing the Phase-4 `RateLimit` stub. FR-BILL-007/009.
- [x] 4.7.3 `usage:push_stripe` daily 02:00 (queue low) → metered plans with a live sub only; `PushUsage(action=set)` + `MarkPushed`; re-runnable via `pushed_at IS NULL`. FR-BILL-007.
- [x] 4.7.4 `billing:reconcile` nightly 04:00 (queue critical) → new `StripeGateway.FetchSubscription` port; drift on (status, plan, period_end, cancel flag) → ERROR log + `billing_reconciliation_drift_total` + heal toward Stripe + cache bust. Stub gateway returns `billing.ErrReconcileUnsupported` ⇒ per-org skip (no remote to drift from) until MODE=live. 06 §8, ADR-010.
- [x] 4.7.5 `Billing.Register(mux)` on the worker + 4 new `jobs.Schedule()` entries (queue opts per 09 §2; maintenance entries moved to queue low).

## Section 8 — Tests

- [x] 4.8.1 Unit: derivation/state machine from 4.2.6 still green; new suites — billing job handlers (drain TaskID/conflict, aggregate day-grain values, metered-only push, reconcile drift/heal/stub-skip w/ prom testutil) + rate-limit middleware (under/over/unlimited/no-tenant). FR-BILL-004.
- [x] 4.8.2 Webhook replay ×5 concurrent → exactly one side-effect set — **first integration-tagged test** (`webhook_repo_integration_test.go`, `-tags=integration`, `TEST_DATABASE_URL`, skips when unset; `make test-integration` supplies the compose DSN). Verified against live docker Postgres. 06 §9.
- [x] 4.8.3 Out-of-order: `updated(T2)` then stale `(T1)` → mirror stays at T2, `last_stripe_event_at` unregressed (same integration file, live-verified). 06 §9.
- [x] 4.8.4 Bad/absent/tampered signature → 400, valid → 200 — HTTP handler test (`handlers/webhooks_test.go`) over the real stub gateway. 06 §9.

## Section 9 — E2E verify (dockerized)

- [x] 4.9.1 [VERIFY] Crafted signed `checkout.session.completed` (stub HMAC, no stripe-cli) → 200, summary pro/active, replay deduped, bad sig 400. FR-BILL-002/005.
- [x] 4.9.2 [VERIFY] `invoice.payment_failed` → `past_due` + warning flag; dunning email landed in Mailpit (proves outbox → drain → email:send → SMTP). FR-BILL-006.
- [x] 4.9.3 [VERIFY] Free org 4th project → `402 plan_limit_exceeded {limit:max_projects, current:3, max:3}`; bonus: free-plan rate limit tripped 429 after ~57 rapid calls. FR-BILL-009.
- [x] 4.9.4 [VERIFY] `scratchpad/smoke4.ps1` (PS 5.1) covers the full flow — ALL GREEN 2026-07-07. Also verified live: `usage:aggregate` populated 13 orgs × 3 metrics in `usage_records`; `billing:reconcile` ran (0 drift, stub skip); `billing_reconciliation_drift_total` exposed on worker :8081/metrics.

## Section 10 — Commit gate

- [x] 4.10.1 [VERIFY] `go build ./... && go vet ./... && go test ./...` green + `-tags=integration` green against the live stack.
- [x] 4.10.2 Committed as a series on `feat/phase2-gap-closure`: §7 jobs, §7 rate limiting, §8 tests, §9–10 e2e + docs. README Current Position advanced.

---

## Definition of Done (Phase 4)

- [x] All BILL-001..009 (M) checks ticked; BILL-010/011 (S/C) optional — not built (seat quantity mirroring TODO(seats) remains, 4.3.3).
- [x] Webhook consumer idempotent + out-of-order safe + signature-verified (4.8.2–4.8.4, integration-verified live).
- [x] Entitlement middleware returns 402 on limit; cache bust on webhook.
- [x] Usage pipeline + reconciliation jobs registered and re-runnable (set-semantics upsert, pushed_at guard, stub-mode reconcile no-op).
- [x] E2E (Section 9) green against dockerized stack; committed. README advanced to Phase 5.
