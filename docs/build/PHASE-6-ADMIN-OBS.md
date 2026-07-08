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

- [x] 6.0.1 [DECISION] Bootstrap = new `cmd/adminctl grant <email>` — idempotent `UPDATE users SET platform_role='admin' WHERE email=$1` on the owner pool. No hardcoded user in seed/migration.
- [x] 6.0.2 [VERIFY] No add needed: `audit_log` (0005) already has `impersonator_user_id`, `severity` (info|warning|security) and nullable `org_id`. → 6.1.5 is a no-op.
- [x] 6.0.3 [DECISION] OpenAPI = `swaggo/swag` **v2** (`github.com/swaggo/swag/v2`, emits OpenAPI 3.1; v1 only does Swagger 2.0). Annotate handlers → `swag init` → serve `/api/v1/openapi.json` + Swagger UI `/api/docs` (non-prod).

## Section 1 — Migrations (0015 DDL, 0016 RLS)

- [x] 6.1.1 SUPERSEDED — `users.platform_role` (+ `totp_secret`/`totp_enabled`) already exist since 0002; not re-added. Instead 0015 adds `plans.monthly_price bigint` (local MRR source, FR-ADM-002; plans otherwise stores only Stripe price IDs).
- [x] 6.1.2 `0015`: `api_keys` [T] (id, org_id, prefix, key_hash UNIQUE, name, scopes text[], created_by, last_used_at, revoked_at, created_at) + `api_keys_org_idx`. FR-API-001.
- [x] 6.1.3 `0015`: `feature_flags` [T] (org_id, flag, enabled, `PK(org_id,flag)`). FR-ADM-006.
- [x] 6.1.4 `0015`: `entitlement_overrides` [T] (org_id, key, value, note, created_by, created_at, `PK(org_id,key)`). FR-ADM-002.
- [x] 6.1.5 NO-OP per 6.0.2 (audit_log already complete).
- [x] 6.1.6 `0015.down.sql` (drops 3 tables + monthly_price) + `0016_admin_rls.up/.down.sql`: RLS `tenant_isolation` on the 3 [T] tables (audit_log stays as-is; app role has no UPDATE/DELETE grant per FR-AUD-002). Seed (`cmd/stripeseed`) sets monthly_price free=0/pro=1200/business=4900.
- [x] 6.1.7 [VERIFY] `migrate up`/`down 2`/`up` clean on 0015/0016 (version 16, no errors).

## Section 2 — Domain

- [x] 6.2.1 `internal/domain/admin/`: `TenantSummary` (+MRR/member count), `TenantFilter`, `WebhookEvent`, `EntitlementOverride`, `FeatureFlag`, `ImpersonationClaim` + ports `AdminRepository`/`FeatureFlagRepository`/`OverrideRepository`. Reuses `auth.PlatformRole` (not redefined).
- [x] 6.2.2 `internal/domain/apikey/`: `APIKey`, `Scope` (read|write, write⇒read); `Generate`/`Hash`/`Verify` (prefix `fbk_live_`+32B hex, SHA-256 hex stored, plaintext once), `LooksLikeKey`. FR-API-001.
- [x] 6.2.3 `apikey/ports.go`: `APIKeyRepository` (Create, List, GetByHash, Revoke, TouchLastUsed).
- [x] 6.2.4 `internal/domain/analytics/`: `DailyStat`, `ProjectAnalytics`, `UsageDashboard` + ports `StatsRepository` (rollup read) + `UsageReader` (Latest/SumRange over usage_records; kept read-only, separate from billing's write-oriented `UsageRepository`). FR-AN-001/002.
- [x] 6.2.5 [VERIFY] `apikey_test.go`: hash/verify (constant-time), scope (write⇒read), generate shape/uniqueness — green.

## Section 3 — Usecase

- [x] 6.3.1 `adminuc`: `ListTenants` (search/plan/status, capped) + `GetTenantDetail` (subscription via `billing.SubscriptionRepository.Get`, webhook events by customer, invoices, overrides, flags). FR-ADM-002.
- [x] 6.3.2 `adminuc.StartImpersonation`: `ImpersonationMinter` port mints the token; records a `security` audit row with BOTH identities. FR-ADM-003. (jwtx `imp` claim + middleware land in §5.)
- [x] 6.3.3 `adminuc.RetryWebhook`: `WebhookRetrier` port enqueues `webhook:retry`; audited. FR-ADM-004.
- [x] 6.3.4 `adminuc`: `ListFlags`/`SetFlag` + `ListOverrides`/`SetOverride`/`DeleteOverride`, all audited. FR-ADM-006/002.
- [x] 6.3.5 `apikeyuc`: `Create` (one-time plaintext), `List`, `Revoke`, `ResolveByKey` (hash→GetByHash→active→touch; misses/revoked ⇒ `ErrUnauthorized`). FR-API-001/002.
- [x] 6.3.6 `analyticsuc`: `ProjectAnalytics` (window series + totals + mean cycle from rollup) + `UsageDashboard` (MTD seats/storage/api-calls + plan base-price estimate). Rollup-only. FR-AN-001/002.
- [x] 6.3.7 `audituc`: `ListForOrg` (forces OrgID isolation) + `ListGlobal`; `export` flag caps rows at `ExportCap` (10k) for CSV. FR-AUD-003/ADM-005. (Added `billing.Plan.MonthlyPrice` field for MRR/estimate.)

## Section 4 — Infra

- [x] 6.4.1 `postgres/api_key_repo.go` — dual-pool: Create/List/Revoke via TenantPool ([T]/RLS); GetByHash/TouchLastUsed on the OWNER pool (auth path, pre-tenant; owner bypasses non-FORCE RLS; hash globally unique).
- [x] 6.4.2 `postgres/feature_flag_repo.go` + `override_repo.go` over TenantPool.
- [x] 6.4.3 `postgres/admin_repo.go` (OWNER pool): `ListTenants`/`GetTenant` with MRR aggregate (`SUM monthly_price` over entitled sub, join subscriptions/plans, member count) + `WebhookEventsForCustomer` (payload `#>>{data,object,customer}`). `analytics_repo.go`: rollup + usage reads via TenantPool.
- [x] 6.4.4 `postgres/audit_read_repo.go`: hand-written dynamic filtered reads on the plain pool (org isolation via explicit org_id predicate), bounded at `ExportCap`.
- [x] 6.4.5 OpenAPI 3.1 (swaggo **v2**, `swag init … --v3.1`): general info on `cmd/api/main.go` + `@Router` annotations on the Phase-6 handlers → `docs/swagger.json`, embedded at `handlers/openapi.json`, served `GET /api/v1/openapi.json` (public) + Swagger UI `GET /api/docs` (non-prod). `make openapi` regenerates; `make gen-client` points the FE codegen at it. Queries `{apikeys,admin,flags,overrides,analytics}.sql` + `monthly_price` on plans; `sqlc generate` clean.

## Section 5 — HTTP

- [x] 6.5.1 `/admin` mounted in `router.go` as a SEPARATE route group, NOT under tenant mw; guard `mw.PlatformAdminGuard.RequireAdmin` = `platform_role=admin` AND `totp_enabled` (login enforces the TOTP step → live session implies 2FA; 6.7.4). Rejects impersonation tokens. FR-ADM-001.
- [x] 6.5.2 `handlers/admin.go`: `ListTenants` (filters) + `GetTenant` detail (sub/invoices/webhooks/overrides/flags). FR-ADM-002.
- [x] 6.5.3 Impersonation: `jwtx.SignImpersonation` (`imp` claim) → `Principal.ImpersonatedOrg`; `TenantGuard.Resolve` grants synthetic read-only ADMIN for the bound org (404 for any other); `mw.ImpersonationReadOnly` 403s writes; start audited with both identities. FR-ADM-003.
- [x] 6.5.4 Admin webhook browser (in detail) + `POST /admin/tenants/{orgId}/webhooks/{eventId}/retry`. FR-ADM-004.
- [x] 6.5.5 `GET /admin/audit` (global) + flag/override PUT/DELETE + `GET /admin/jobs` (JobsInspector port). FR-ADM-005/006.
- [x] 6.5.6 `middleware/apikey.go`: `HybridAuth` resolves `Bearer fbk_…` → org+scopes (no session); `TenantGuard` maps scope→synthetic role; `APIKeyScopeGuard` 403s writes from read keys; `X-RateLimit-Limit` header on org responses; plan rate limit reused (429+Retry-After). FR-API-002.
- [x] 6.5.7 Org `GET/POST /api-keys` (one-time secret reveal), `DELETE /api-keys/{id}` (O(ADMIN)); org audit `GET /audit` + `GET /audit.csv` (O(ADMIN), 10k cap). FR-API-001, FR-AUD-003.
- [x] 6.5.8 Analytics `GET /projects/{id}/analytics` (O(MEMBER)) + `GET /usage` (O(ADMIN)); OpenAPI `GET /api/v1/openapi.json` (public) + Swagger UI `GET /api/docs` (non-prod, `DevDocs`). FR-AN-001/002, FR-API-003.

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
