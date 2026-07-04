# 03 — System Architecture

## 1. C4: Container Diagram (textual)

```
[Browser SPA — Next.js 14]
   │ HTTPS JSON (access JWT in Authorization header;
   │ refresh token in httpOnly cookie, only sent to /auth/refresh)
   ▼
[Go API  :8080] ──────────────┬────────────► [PostgreSQL 16]  (primary data, RLS)
   │  chi router               │
   │  middleware chain:        ├────────────► [Redis 7]       (sessions cache, rate limit,
   │   requestID → logging     │                                usage counters, SSE streams,
   │   → recover → CORS        │                                Asynq queues, entitlement cache)
   │   → auth → tenant         │
   │   → RBAC → entitlement    ├────────────► [MinIO]         (attachments, avatars, logos —
   │   → rate limit            │                                presigned PUT/GET, bucket: fluxboard)
   │                           │
   │                           └────────────► [Stripe API]    (customers, subscriptions,
   │                                                            checkout, portal, usage records)
   │  enqueues jobs
   ▼
[Go Worker — Asynq] ◄──────── [Redis queues]
   │  email send (SMTP→Mailpit local)
   │  usage aggregation (hourly) / usage push to Stripe (daily)
   │  stats rollup (nightly), orphan-upload GC, trash purge,
   │  org hard-delete after grace, audit retention purge
   ▼
[Prometheus] scrapes /metrics of api+worker ──► [Grafana] provisioned dashboards

[Stripe] ──webhooks──► /api/v1/webhooks/stripe (signature-verified; local dev:
                        stripe-cli container forwards events)
```

Two deployables from one Go module: `cmd/api` and `cmd/worker`. They share
`internal/` code; the worker imports usecases, never HTTP handlers.

## 2. Request Middleware Chain (order is contract)

1. `RequestID` — UUIDv7 into context + `X-Request-ID` response header
2. `Logger` — slog JSON: method, path, status, duration, request_id, user_id, org_id
3. `Recoverer` — panic → 500 + stack to log (never to client)
4. `CORS` — allowlist of web origin(s)
5. `Auth` — parse Bearer: JWT path (validate ES256, load session state from Redis→PG fallback, reject revoked `sid`) OR API-key path (`fbk_` prefix → hash lookup)
6. `TenantResolver` — org from URL param (session auth) or key binding (API-key auth); verifies membership; acquires **tenant-scoped DB connection** (`SET LOCAL app.current_tenant` — see 05 §4); injects `TenantContext{OrgID, UserID, OrgRole, AuthKind}`
7. `RBAC` — Casbin enforce (route-declared object/action)
8. `Entitlement` — plan/limit checks for write routes (06 §7)
9. `RateLimit` — Redis sliding window keyed by user/org/API-key per plan tier

## 3. Backend Folder Structure

```
backend/
├── cmd/
│   ├── api/main.go            # DI wiring, server, graceful shutdown (SIGTERM, 20s drain)
│   └── worker/main.go         # Asynq server, task handler registration
├── internal/
│   ├── domain/                # ZERO external deps (stdlib only)
│   │   ├── auth/              # User, Session, RefreshToken, OAuthIdentity + repo interfaces
│   │   ├── tenant/            # Organization, Membership, Invitation, Role
│   │   ├── project/           # Project, Board, Column, Task, Subtask, Label, Comment, Attachment
│   │   ├── billing/           # Subscription, Plan, Invoice, UsageRecord, Entitlements
│   │   ├── notification/      # Notification, Preference
│   │   ├── audit/             # AuditEntry
│   │   └── errors.go          # ErrNotFound, ErrForbidden, ErrConflict, ErrPlanLimit, ErrValidation
│   ├── usecase/               # application services; depend on domain interfaces only
│   │   ├── authuc/  tenantuc/  projectuc/  taskuc/  billinguc/  notifyuc/  adminuc/
│   ├── infrastructure/
│   │   ├── postgres/
│   │   │   ├── queries/       # *.sql — source of truth for sqlc
│   │   │   ├── gen/           # sqlc output (committed, never hand-edited)
│   │   │   ├── tenantpool.go  # RLS connection wrapper (05 §4)
│   │   │   └── *_repo.go      # repository implementations
│   │   ├── redis/             # sessioncache, ratelimit, usagecounter, sse streams, entitlement cache
│   │   ├── stripe/            # client wrapper + webhook event mapper
│   │   ├── minio/             # presign service
│   │   ├── mailer/            # SMTP + html templates (embed.FS)
│   │   └── casbinx/           # model.conf, pg adapter, enforcer bootstrap
│   ├── interface/
│   │   ├── http/
│   │   │   ├── handlers/      # one file per resource
│   │   │   ├── middleware/
│   │   │   ├── errmap.go      # domain sentinel → HTTP status (single place)
│   │   │   ├── request/ response/   # DTOs + validation (go-playground/validator)
│   │   │   └── router.go
│   │   └── jobs/              # Asynq task payloads + handlers
│   └── pkg/                   # jwtx, argon2x, rank (LexoRank), token (random gen), clock
├── migrations/                # NNNN_name.up.sql / .down.sql (golang-migrate)
├── sqlc.yaml
└── Makefile
```

**Dependency rule (enforced by `go-arch-lint` in CI):**
`domain ← usecase ← interface`, `domain ← infrastructure`. `usecase` never
imports `infrastructure`; wiring happens only in `cmd/*/main.go`.

## 4. Frontend Structure

```
web/
├── app/
│   ├── (public)/  (auth)/  (app)/  (admin)/     # route groups per 02-SITEMAP.md
│   ├── layout.tsx  globals.css
├── components/ui/            # shadcn
├── components/board/         # kanban: Board, Column, TaskCard, useDndHandlers
├── components/billing/  components/shared/
├── lib/
│   ├── api/gen/              # openapi-typescript generated client
│   ├── api/client.ts         # fetch wrapper: attaches access token, 401 → single-flight
│   │                         # refresh (mutex) → retry once → hard logout
│   ├── auth/                 # AuthProvider (token in memory), useSession
│   └── sse.ts                # EventSource wrapper, Last-Event-ID, backoff reconnect
├── hooks/                    # useTasks, useBoard (TanStack Query keys per org/project)
└── middleware.ts             # edge: redirect unauthenticated (app) → /login (cookie presence
                              # heuristic only; real authz is always server-side)
```

## 5. Key Runtime Flows

**Kanban move (FR-PROJ-005):** client computes new rank between neighbors →
optimistic cache update → `PATCH /tasks/{id}/position {column_id, rank}` →
server validates column ∈ project, writes one row, emits `task.moved` to org
Redis Stream → SSE fan-out; other clients reconcile. Rank collision → 409 →
client refetches column and retries once with a rebalanced key.

**Checkout (FR-BILL-002):** UI → `POST /billing/checkout {plan}` → usecase
creates/gets Stripe customer, creates Checkout Session (idempotency key
`co:{orgID}:{plan}:{floor(now/300)}`) → redirect URL → user pays on Stripe →
redirect to success page which POLLS local subscription state →
`checkout.session.completed` webhook is the source of truth that flips the
org to the paid plan and invalidates the entitlement cache.

## 6. Architecture Decision Records (summary — keep full ADRs in /docs/adr)

| ADR | Decision | Rationale / rejected alternative |
|-----|----------|----------------------------------|
| 001 | Go backend, not Node/Nest | Goroutine model fits webhook/job concurrency; compile-time types end-to-end with sqlc; differentiates portfolio (existing TS projects). Rejected: NestJS (already demonstrated in task-management project). |
| 002 | Modular monolith, not microservices | Single deployable ×2 processes; module boundaries via Go packages + dependency lint. Microservices add distributed-transaction pain with zero benefit at this scale; interview answer: "boundaries first, network later". |
| 003 | sqlc + pgx, not GORM/ent | Hand-written SQL under version control, compile-time-checked; enables RLS `SET LOCAL` control and honest index/EXPLAIN discussions. Rejected: GORM (query opacity, weak RLS ergonomics). |
| 004 | PostgreSQL RLS + app-level filtering (both) | Defense in depth; a forgotten WHERE clause fails closed. Cost: connection-scoped GUC discipline (05 §4). Rejected: schema-per-tenant (migration fan-out, connection bloat at N tenants). |
| 005 | Casbin, not hand-rolled RBAC | Declarative model file makes the RBAC+ABAC hybrid (org role + project membership + resource ownership) reviewable; battle-tested Go library. Rejected: middleware if-chains (untestable matrix). |
| 006 | SSE, not WebSocket | Traffic is server→client only; SSE gives auto-reconnect + Last-Event-ID replay natively, plain HTTP/2, no ws upgrade infra. WebSocket revisited only if client→server streaming appears (ADR to be reopened). |
| 007 | Opaque refresh token in httpOnly cookie; JWT access in memory | XSS cannot exfiltrate refresh token; CSRF surface limited to /auth/refresh (SameSite=Strict + custom header check). Rejected: JWT-as-refresh (revocation requires denylist anyway → opaque is simpler and honest). |
| 008 | No role/tenant claims inside access JWT | Role changes must apply immediately, not at token expiry; per-request resolution with Redis cache (60s) costs ~0.3ms. Rejected: fat tokens (stale-privilege window = security finding). |
| 009 | LexoRank-style string ranks for ordering | O(1) row writes per move vs integer positions (O(n) reshuffle) — matters under concurrent board edits. Rebalance job when key length > 64. |
| 010 | Local mirror of Stripe state | Page loads never call Stripe synchronously; webhook-driven sync + nightly reconciliation job (06 §8) detects drift. Rejected: Stripe-as-database (latency, rate limits, outage coupling). |
| 011 | Asynq over hand-rolled Redis queues | At-least-once with retries/backoff, scheduled jobs, dead-letter, asynqmon UI. Handlers must be idempotent (documented per task in 09). |
| 012 | Money as int64 minor units | Float currency is a defect class, not a preference. Matches Stripe's API shape. |
