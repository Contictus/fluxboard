# 12 — Testing Strategy

## 1. Test Pyramid & Targets

| Layer | Tooling | Scope | Coverage target |
|---|---|---|---|
| Unit (Go) | stdlib testing + testify | domain logic, rank math, token gen, entitlement derivation, JWT/Argon2 params | usecase ≥ 85%, billing pkg ≥ 95% |
| Integration (Go) | testcontainers-go (pg, redis, minio) | repositories against real PG (RLS active!), webhook consumer, jobs | all repos + consumer |
| API contract | Go httptest against wired router | every endpoint: happy + authz + validation + error envelope | every route ≥ 1 authz-negative test |
| E2E | Playwright | 6 golden journeys (below) | smoke in CI, full nightly |
| Load/abuse | k6 | board under concurrency, rate-limit behavior, SSE fan-out | profiles recorded in README |

Integration tests run against **RLS-enabled** schema with the `fluxboard_app`
role — testing with a superuser would silently bypass the exact layer under test.

## 2. Golden E2E Journeys (Playwright)

1. Register → verify (Mailpit API fetch) → create org → invite member →
   second browser accepts → both see each other
2. Create project → create 5 tasks → drag between columns → second browser
   sees moves live (SSE) → comment with @mention → notification appears
3. Upgrade to Pro via Stripe Checkout (test card 4242…) → success page
   resolves after webhook → project limit raised
4. Payment failure path: `stripe trigger invoice.payment_failed` → past_due
   banner → portal link present
5. OAuth login (mocked Google via local OIDC stub container) → identity linked
6. Admin: impersonate tenant read-only → write attempt rejected → audit
   entries carry both identities

## 3. Unit-Test Focal Points

- LexoRank: between/append/prepend, exhaustion → rebalance trigger, property
  test (10k random moves keep total order)
- Entitlements: (plan × status) matrix → expected Entitlements struct (table test)
- Refresh rotation state machine: valid→rotate, rotated→family-revoke,
  expired→revoke, concurrent (2 goroutines, real PG row lock in integration tier)
- Cycle detection for task relations (path returned)
- Argon2id/JWT parameter regression guards (assert configured values)

## 4. Auth Security Suite (maps to 04 §6)

- Timing: unknown-email vs wrong-password login deltas statistically
  indistinguishable (n=200, Welch t-test tolerance)
- Response-body equality on forgot-password (known vs unknown email)
- Token single-use: parallel double-consume of reset token → exactly one succeeds
- Session revocation latency ≤ cache TTL (revoke → API call 401 within 60 s, in tests: immediate via cache-bust assertion)

## 5. Seed Data (`make seed`)

Deterministic fixture (fixed UUIDs, fixed clock):
- Users: `owner@demo.dev`, `admin@…`, `member@…`, `guest@…`, `outsider@…` (all password `Demo-Pass-2026!`), `padmin@…` (platform admin, TOTP seeded)
- Orgs: `acme` (Business, active, 20 members, 6 projects, 400 tasks, usage history 30d), `beta-co` (Free, 2 members), `pastdue-inc` (Pro, past_due)
- One project per state: active/archived/private; tasks distributed across columns with realistic timestamps (feeds analytics rollup verification)

## 6. Tenant Isolation Suite (Phase-2 gate — must be exhaustive, generated)

Test generator iterates **every tenant-scoped endpoint** (route table
introspection) × principal in {member-of-A, admin-of-A, outsider} attempting
org-B resources: expected 404/403 per matrix. Plus:
- Raw repository call with GUC unset → 0 rows (fail-closed proof)
- INSERT with mismatched org_id under RLS `WITH CHECK` → error
- Search/FTS endpoint never returns foreign-org hits (seeded marker strings)
- Presigned download for foreign-org attachment id → 404 before presign

## 7. Billing Reliability Suite (Phase-4 gate)

- Same `checkout.session.completed` payload delivered 5× concurrently
  (goroutines against consumer) → exactly one subscription flip, one welcome
  email outbox row
- Out-of-order: `subscription.updated(created=T2)` then stale `T1` → mirror
  remains T2
- Signature: tampered body / stale timestamp / wrong secret → 400, nothing recorded
- Consumer crash simulation: side-effect tx forced to fail → 500 → redelivery
  → succeeds → single effect
- Proration preview vs applied invoice consistency (Stripe test clock advance)
- Metering: rerun `usage:push_stripe` for same day → Stripe quantity unchanged
- Reconciliation: mutate mirror manually → job heals + drift counter increments

## 8. Jobs & Realtime Suite (maps 09 §4)

Worker kill/rerun idempotency per task type; outbox backlog drain without
email duplicates (Mailpit count assertion); SSE `Last-Event-ID` replay
correctness; `resync` on over-aged cursor; 500-connection fan-out k6 profile.

## 9. CI Wiring

`make test` = unit; `make test-integration` = testcontainers suites (tenant
isolation + billing suites are REQUIRED status checks); Playwright smoke
(journeys 1–3) on PR, full set nightly. Flake policy: quarantine label +
issue, never retry-until-green as a merge strategy.
