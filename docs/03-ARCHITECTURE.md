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
| 013 | Casbin models the static role→permission matrix only; the user↔role↔org mapping stays in the `memberships` table (single source of truth) | Avoids per-org `p` policy seeding + `g`-fact dual-writes (a consistency hazard the 05 §3 sketch acknowledges). TenantResolver resolves the caller's role from `memberships`; the enforcer answers only "role → (object, action)". Role inheritance (OWNER>ADMIN>MEMBER>GUEST) states each permission once. Rejected: per-org policies (table explosion) and Casbin as the membership store (dual-write drift vs the DB). |
| 014 | Phase-2 tenant tables use `ENABLE` (not `FORCE`) RLS + two audited `SECURITY DEFINER` functions for the unavoidable cross-tenant reads | `fluxboard_app` is `NOBYPASSRLS` and never owns the tables, so `ENABLE` alone fully isolates it (verified: GUC-unset ⇒ 0 rows). `FORCE` would also subject the owner, breaking the `SECURITY DEFINER` "my orgs" (`app_current_user_orgs`) and invite-by-token (`app_invitation_by_token`) reads, which must see across tenants by design. Both functions expose only a narrow projection and are `EXECUTE`-granted to the app role. Deviates from 05 §4's `FORCE` for these two tables only; revisit if the app ever runs as the table owner. |
| 018 | Phase 7 §6 task detail + search/trash/my-tasks composes over the flat `/tasks/{taskId}` API; three sitemap↔API gaps are surfaced rather than improvised | The task surface needed no new endpoints, but three mismatches with 02-SITEMAP were surfaced (not worked around silently): **(a)** task detail URLs are `/projects/{projectKey}/tasks/{taskNumber}`, but tasks are addressed by UUID `{taskId}` only and `tasks/search` carries **no `number` filter** — so number→id resolves through the **board projection** (`GET .../board` lists every card's `number`+`id`), the same cache the board page already holds (`lib/board/use-task.ts`); a task absent from the board (e.g. trashed) 404s the detail page, which is correct. **(b)** The sitemap's task detail is an *intercepted modal over the board with a full page on direct load*; this ships the **full page for both** (the card links there) and **defers the modal interception** — a Next parallel/intercepting route over the client board adds fragility for little gain now, and the full page is the source of truth either way. **(c)** `/trash` and `/my-tasks` are **org-level** in the sitemap, but the API exposes trash **per project** (`GET .../projects/{id}/trash`) and no `assignee=me` alias — so the org trash view **fans out ListTrash across every project and flattens**, and my-tasks passes the caller's own id to search (per ADR-016). There is **no hard-purge endpoint** (trash auto-purges via the retention job), so `/trash` is **restore-only** with explanatory copy, not a purge action. Also: the `board`/task card DTOs carry only user **ids**, so assignee display, the assignee picker and @mention autocomplete are hydrated from `GET /orgs/{id}/members` (`lib/org/use-members.ts`); @mention suggests **org** members (the project-member set is a subset — project-scoped filtering can layer on later via `/projects/{id}/members`). Comment bodies render as pre-wrapped text (a full markdown renderer is out of scope this section). |
| 017 | Phase 7 §5 projects/kanban composes over the existing project API; three gaps are surfaced rather than improvised, and LexoRank is ported to the client | The board area needed no new endpoints, but three mismatches between 02-SITEMAP and the API were surfaced (not silently worked around): **(a)** projects are addressed by UUID `{projectId}` only — there is **no by-key route** — so the `/projects/{projectKey}` URLs resolve key→UUID client-side via `listProjects().find(p => p.key === key)` (one cached list, 404 UI if absent); **(b)** `CreateProject` seeds a fixed default board (`DefaultColumns`) and takes **no template param**, so the create form drops sitemap 7.5.2's "template" field — new boards start with server defaults and columns are edited in project settings; **(c)** board filters are **client-side** over the loaded board projection and cover **text/priority/assignee only** — the `board` card DTO (`taskCardResp`) carries no label ids, so a label filter is impossible without N extra calls (labels remain usable for the bulk **add-label** action, which needs only ids). **LexoRank client port:** `PATCH /tasks/{id}/position` stores the client-supplied `rank` verbatim and 409s on collision, so the drag-drop mints the key in the browser. `web/lib/board/rank.ts` is a faithful port of `backend/internal/pkg/rank/rank.go` (base-36 `0-9a-z`, `mid="i"`, byte order == sort order) — validated with a monotonicity torture test mirroring `rank_test.go` — guaranteeing a client key sorts identically to Postgres `ORDER BY rank`, so a move needs no server round-trip to place a card. Optimistic cache update on drop; on error (esp. 409) the snapshot is restored and the board invalidated. |
| 016 | Phase 7 §4 org shell wires to existing endpoints; five home/edge features are composed from what the API already exposes rather than new endpoints | The shell (`/app/{orgSlug}`) needs role + soft-delete state + a home feed, but no single endpoint carries them. Chosen composition (surfaced, not improvised): **(a)** org context = `GET /orgs` (only source of the caller's per-org role) **+** `GET /orgs/{id}` (only source of `deleted_at`/`logo_key`), joined client-side by slug; **(b)** org-home "recent activity" reuses the caller's own `GET /orgs/{id}/notifications` — there is no org-wide, member-visible activity feed (the audit log is ADMIN-only, so unsuitable); **(c)** "pinned projects" renders the most-recent active projects from `GET /orgs/{id}/projects` — the schema has no pin/recency concept; **(d)** the soft-delete grace screen shows a restore CTA with qualitative copy but **no live countdown**, because `orgResp` exposes `deleted_at` but not the domain's `purge_after` (recommended follow-up: add `purge_after` to `orgResp` for a real countdown); **(e)** the `past_due` banner fetches `GET /orgs/{id}/billing/summary`, which is `read:billing` = ADMIN+, so it is fetched/rendered only for OWNER/ADMIN (a MEMBER/GUEST would 403) — consistent with 02 §9 ("banner for OWNER/ADMIN"). Also note: assigned-to-me passes the caller's own id to `tasks/search?assignee_id=` since there is no server-side `assignee=me` alias. |
| 015 | Account self-service (`/me`) built in Phase 7 as a dedicated `useruc` usecase; global notification prefs (08 §3) **not** built | The `docs/08 §3` account endpoints (`GET/PATCH /me`, avatar presign, `DELETE /me`) were specced but never implemented in Phases 1–6; the frontend needs them, so they land now. `useruc` is org-independent (no tenant pool); profile reuses `users.avatar_key` (already in 0002) + the MinIO store; `DELETE /me`'s sole-owner guard runs a cross-org read on the **owner pool** (memberships RLS is non-FORCE, ADR-014), returning `409 sole_owner_of` — so account deletion is disabled when `DATABASE_URL_MIGRATE` is unset, consistent with the Phase-6 admin surfaces. **Rejected: global (org-less) notification prefs** — `notification_prefs` is deliberately per-`(org,user,category)` `[T]` with RLS (0013 rejects the 07 §4 org-less shape for invariant #1), so the account "notifications" page composes the existing per-org prefs endpoints with an org selector rather than a new global store. Email-change (`one_time_tokens.purpose='email_change'` already exists) is deferred to a follow-up; the profile UI shows the control disabled until then. |
