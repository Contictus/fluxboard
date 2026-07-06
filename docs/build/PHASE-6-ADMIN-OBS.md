# Phase 6 — Platform Admin, Audit Viewer, API Keys, Analytics, Observability

**Modules:** ADM-001..006 · AUD-003 (+AUD-001/002 wiring) · API-001..004 · AN-001..002 ·
**Spec:** `docs/01-…md` §ADM/AUD/API/AN, `docs/10-INFRA-DEVOPS.md`, `docs/11-SECURITY.md`.
**Depends on:** Phase 4 (billing mirror for MRR/webhooks) + Phase 5 (rollup job).
**Gate:** admin surface + API keys + analytics e2e green.

> This phase is the operator + platform layer: a separate `/admin` Go router
> (never under tenant middleware), org-scoped API keys as a second auth path,
> analytics served from nightly rollups (not live aggregates), and the metrics/
> dashboards that make it observable.

---

## Section 0 — Prereqs & decisions

- [ ] 6.0.1 [DECISION] How the first `platform_role=admin` is bootstrapped (seed migration / CLI). Record here.
- [ ] 6.0.2 [VERIFY] Audit `audit_log` (0005) columns vs FR-AUD-001 (needs `impersonator_user_id`, `severity`, nullable `org_id`); note any add for 6.1.5.
- [ ] 6.0.3 [DECISION] OpenAPI generation approach (annotation lib e.g. `swaggo/swag` vs maintained hand-written 3.1 spec). Record. FR-API-003.

## Section 1 — Migrations (0015 DDL, 0016 RLS)

- [ ] 6.1.1 `0015_admin.up.sql`: `users.platform_role` column (null | 'admin'). FR-ADM-001.
- [ ] 6.1.2 `0015`: `api_keys` [T] (id, org_id, prefix, key_hash, name, scopes text[], created_by, last_used_at, revoked_at, created_at). FR-API-001.
- [ ] 6.1.3 `0015`: `feature_flags` [T] (org_id, flag, enabled, `PK(org_id,flag)`). FR-ADM-006.
- [ ] 6.1.4 `0015`: `entitlement_overrides` [T] (org_id, key, value, note, created_by, created_at). FR-ADM-002.
- [ ] 6.1.5 `0015`: add any missing `audit_log` columns from 6.0.2. FR-AUD-001.
- [ ] 6.1.6 `0015.down.sql` + `0016_admin_rls.up/.down.sql`: RLS `tenant_isolation` on `api_keys`, `feature_flags`, `entitlement_overrides` (audit_log stays as-is; app role has no UPDATE/DELETE grant per FR-AUD-002).
- [ ] 6.1.7 [VERIFY] `migrate up`/`down 2`/`up` clean on 0015/0016.

## Section 2 — Domain

- [ ] 6.2.1 `internal/domain/admin/`: `PlatformRole`, `TenantSummary`, `EntitlementOverride`, `FeatureFlag`, `ImpersonationClaim` (imp) models + ports (`AdminRepository`, `FeatureFlagRepository`, `OverrideRepository`).
- [ ] 6.2.2 `internal/domain/apikey/`: `APIKey`, `Scope` (read|write) models; hashing contract (prefix `fbk_live_` + 32B random, SHA-256 stored, plaintext shown once). FR-API-001.
- [ ] 6.2.3 `apikey/ports.go`: `APIKeyRepository` (create, get-by-hash, list, revoke, touch-last-used).
- [ ] 6.2.4 `internal/domain/analytics/`: `ProjectStats`, `UsageDashboard` views + ports (`StatsRepository` read, reuse billing `UsageRepository`). FR-AN-001/002.
- [ ] 6.2.5 [VERIFY] Unit: API-key hash+verify + scope check.

## Section 3 — Usecase

- [ ] 6.3.1 `adminuc`: tenant list (search, plan/status filters, member count, MRR) + tenant detail (subscription timeline, webhook events for customer, overrides). FR-ADM-002.
- [ ] 6.3.2 `adminuc`: impersonation — mint short-lived token with `imp` claim (read-only); dual-identity context. FR-ADM-003.
- [ ] 6.3.3 `adminuc`: webhook browser (list processed_stripe_events) + retry (enqueue `webhook:retry`). FR-ADM-004.
- [ ] 6.3.4 `adminuc`: feature flags CRUD + entitlement override CRUD. FR-ADM-006/002.
- [ ] 6.3.5 `apikeyuc`: create (return one-time plaintext), list, revoke; `ResolveByKey` (hash → org context + scopes). FR-API-001/002.
- [ ] 6.3.6 `analyticsuc`: project analytics (completed/week, cumulative flow, cycle time, per-assignee) + org usage dashboard (seats/storage/api-calls, invoice estimate). Reads rollup tables only. FR-AN-001/002.
- [ ] 6.3.7 `audituc`: org-scoped viewer (filters actor/action/date, CSV export ≤10k, FR-AUD-003) + global viewer (FR-ADM-005).

## Section 4 — Infra

- [ ] 6.4.1 `postgres/api_key_repo.go` over TenantPool.
- [ ] 6.4.2 `postgres/feature_flag_repo.go` + `override_repo.go`.
- [ ] 6.4.3 `postgres/admin_repo.go`: tenant list with MRR aggregate (join subscriptions/plans), webhook events by customer.
- [ ] 6.4.4 `postgres/audit_read_repo.go`: filtered reads + CSV streaming (bounded 10k).
- [ ] 6.4.5 OpenAPI 3.1 generation per 6.0.3 → serve `/api/v1/openapi.json`; hook `make gen-client` for the frontend types. FR-API-003. Add queries `queries/{apikeys,admin,flags,audit_read}.sql`; `sqlc generate`.

## Section 5 — HTTP

- [ ] 6.5.1 `internal/interface/http/admin/router.go`: SEPARATE chi router mounted at `/admin`, NOT under tenant middleware; guard = `platform_role=admin` + mandatory TOTP. FR-ADM-001.
- [ ] 6.5.2 Admin handlers: tenants list + detail. FR-ADM-002.
- [ ] 6.5.3 Admin: impersonation start (banner claim); a global write-guard rejects any write when `imp` claim present; every impersonated request audit-logged with both identities. FR-ADM-003.
- [ ] 6.5.4 Admin: webhook event browser + retry button endpoint. FR-ADM-004.
- [ ] 6.5.5 Admin: global audit viewer + feature-flag matrix + Asynq jobs summary (`/admin/jobs`). FR-ADM-005/006.
- [ ] 6.5.6 `middleware/apikey.go`: `Authorization: Bearer fbk_…` path resolves org from key (no user session); per-key rate limit by plan tier (Redis sliding window; `X-RateLimit-*` headers; 429 + Retry-After). FR-API-002.
- [ ] 6.5.7 Org settings API-key endpoints (`/orgs/{orgId}/api-keys` O(ADMIN), one-time reveal, revoke) + org audit viewer + CSV (O(ADMIN)). FR-API-001, FR-AUD-003.
- [ ] 6.5.8 Analytics endpoints (project analytics O(MEMBER), org usage O(ADMIN)) + Swagger UI `/api/docs` (non-prod). FR-AN-001/002, FR-API-003.

## Section 6 — Wiring

- [ ] 6.6.1 `cmd/api/main.go`: mount admin router; wire `adminuc`/`apikeyuc`/`analyticsuc`/`audituc`.
- [ ] 6.6.2 Insert API-key auth as an alternate branch in the security chain (session OR api-key resolves org context).
- [ ] 6.6.3 `api_calls` INCR in rate-limit middleware → feeds usage metering (FR-API-004 → FR-BILL-007) + usage dashboard.

## Section 7 — Observability (`docs/10-INFRA-DEVOPS.md`)

- [ ] 6.7.1 Expand Prometheus metrics: HTTP request duration histogram (by route/status), Asynq job counters, `billing_reconciliation_drift_total`, SSE subscribers gauge.
- [ ] 6.7.2 Provision Grafana dashboards under `deploy/grafana/` (API latency, job throughput, billing health).
- [ ] 6.7.3 [VERIFY] `audit:retention` job (built Phase 5) honors per-plan retention via privileged role. FR-AUD-002.
- [ ] 6.7.4 [VERIFY] Platform-admin TOTP enforcement (no admin route reachable without 2FA). FR-ADM-001, 11-SECURITY.
- [ ] 6.7.5 `/metrics` restricted to internal network; `/readyz` covers DB+Redis (+MinIO/Stripe optional).

## Section 8 — Tests

- [ ] 6.8.1 API-key hash/scope/rate-limit unit.
- [ ] 6.8.2 Impersonation: writes → 403; dual-identity audit row asserted.
- [ ] 6.8.3 Audit CSV export capped at 10k rows.
- [ ] 6.8.4 Analytics correctness: rollup output matches a raw recompute on seeded data.

## Section 9 — E2E verify (dockerized)

- [ ] 6.9.1 [VERIFY] Admin login (platform_role + TOTP) → `/admin/tenants` list + detail with MRR/webhooks.
- [ ] 6.9.2 [VERIFY] Impersonate an org → reads OK, a write returns 403, audit shows both identities.
- [ ] 6.9.3 [VERIFY] Create API key (one-time reveal) → call `/api/v1/...` with `Bearer fbk_…` → scoped access, `X-RateLimit-*` headers present.
- [ ] 6.9.4 [VERIFY] Project analytics + org usage dashboard return rollup data; `/api/v1/openapi.json` served. Write `scratchpad/smoke6.ps1`.

## Section 10 — Commit gate

- [ ] 6.10.1 [VERIFY] `cd backend && go build ./... && go vet ./... && go test ./...` green.
- [ ] 6.10.2 `git commit` (`feat(admin): phase 6 — platform admin, api keys, analytics, observability`); update README Current Position.

---

## Definition of Done (Phase 6)

- [ ] ADM-001..005, AUD-003, API-001..004, AN-001..002 (M) ticked; ADM-006 (S) optional.
- [ ] `/admin` isolated from tenant mw + TOTP-gated; impersonation read-only + audited.
- [ ] API-key auth path works, rate-limited, feeds usage; OpenAPI served + client-gen wired.
- [ ] Analytics from rollups; Prometheus/Grafana observability in place.
- [ ] E2E green; committed. README advanced to Phase 7.
