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
| 022 | Phase 7 §10 platform-admin + analytics wires to the existing `/admin` + analytics endpoints; five sitemap↔API gaps are surfaced rather than improvised | The admin surface needed no new endpoints, but five mismatches with 02-SITEMAP §10 were surfaced (not silently worked around): **(a)** there is **no KPI/metrics endpoint** — the `/admin` dashboard derives its headline numbers (org count, total MRR, paying orgs, member total, plan/status breakdown) **client-side** by reducing `GET /admin/tenants` (fetched with a high limit); **(b)** there is **no `/admin/users` endpoint** — the admin API is tenant-centric (`/admin/tenants[...]`), so the sitemap's users page is **dropped** (a cross-tenant user directory would need a new endpoint); **(c)** there are **no standalone `/admin/webhooks` or `/admin/flags` pages** — webhook events, feature flags and entitlement overrides are all **per-tenant** and returned by `GET /admin/tenants/{orgId}` (raw domain structs), so webhook **retry**, flag **toggle** and override **CRUD** live inside the tenant detail, not as global lists; **(d)** the tenant-detail JSON **mixes casing** — `tenant` is a snake_case DTO but `overrides`/`flags`/`webhooks`/`subscription`/`invoices` are **raw domain structs with no json tags → PascalCase keys** — so the frontend types (`AdminOverride`/`AdminFlag`/`AdminWebhook`/`AdminSubscription`/`AdminInvoice`) match the wire verbatim, PascalCase and all (recommended backend follow-up: give these DTOs json tags for consistency); **(e)** project analytics has **no per-assignee series** — `GET /projects/{id}/analytics` rolls up by day + `column_snapshot` only, so the analytics page ships throughput (created vs completed), cumulative flow (stacked columns) and cycle time, but **not** the sitemap's per-assignee chart. Also: the admin shell is gated on `platform_role==='admin'` read from `/me` (the JWT deliberately omits platform_role, docs/04); every `/admin` call is *additionally* 2FA-gated server-side. **Impersonation:** the detail page mints a read-only token (`POST .../impersonate`), adopts it into the in-memory store, and navigates to the tenant's org shell; the app-wide banner keys off the `imp` claim; **Exit** just `refreshSession()`s — the admin's refresh **cookie** was never touched (only the in-memory access token was swapped), so a refresh restores the real identity (invariant #4 / ADR-021 token hygiene). |
| 021 | Phase 7 §9 realtime client uses `fetch`+ReadableStream, not native EventSource, because the access token is memory-only | The SSE endpoint (`GET /orgs/{id}/events`) authenticates with `Authorization: Bearer` like every other API route (verified: the org group runs `Authenticator.Authenticate`, which reads the bearer header only — no query-param or cookie token path). The browser's native **`EventSource` cannot set request headers**, and our access token lives ONLY in memory — never a cookie or a URL query param (invariant #4 / docs/CLAUDE.md: card-data/token hygiene, and tokens in URLs leak via logs/referers). So `web/lib/sse.ts` consumes the `text/event-stream` with `fetch()` + a `ReadableStream` reader, attaching the bearer and the `Last-Event-ID` header, parsing SSE frames by hand, with 1→2→5→10s backoff (+jitter), a 401→silent-refresh→reconnect path, and heartbeat/`resync` handling. Rejected: (i) a query-param token (leaks the credential); (ii) switching the SSE route to cookie auth (would fork the auth model for one endpoint and reintroduce CSRF surface). The `Realtime` bridge applies **surgical** cache invalidations from each event's `project_id`/`task_id` payload rather than a blanket refetch, and skips events whose `actor_id` is the caller (the optimistic mutation already updated the cache). Note: notification rows are **not deep-linked** to task detail — a task notification carries only the task UUID, and the pretty `/projects/{key}/tasks/{number}` URL needs the board projection to resolve (same constraint as ADR-018); marking read is the row action. |
| 020 | Phase 7 §8 billing UI wires to the existing billing/analytics endpoints; three sitemap↔API gaps are surfaced rather than improvised | The billing surface (`/billing/*`) needed no new endpoints, but three mismatches with 02-SITEMAP were surfaced (not silently worked around): **(a)** there is **no public plan-catalog / pricing endpoint** — the plan tiers (free/pro/business), their entitlement limits, and per-tier ordering are mirrored **client-side** in `web/lib/billing/plans.ts` (kept in sync with `backend/internal/domain/billing`) for display only; the **authoritative** cost of switching an active subscription comes from `POST /billing/preview-change` (proration, shown in a confirm dialog before `POST /billing/change`), and the checkout amount from Stripe's hosted page — the UI never invents a price. **(b)** The billing summary DTO carries **no payment-method last4** (card data never transits our servers, invariant #4), so the overview shows no card digits; "Manage payment method" opens the **Stripe Billing Portal** (`POST /billing/portal`) instead. **(c)** `GET /usage` returns **current-period aggregates (a snapshot)**, not a daily time series, so `/billing/usage` renders **usage-vs-limit meters + a Recharts bar for API calls + the estimated total**, not a 30-day chart (the real daily series exists only for *project* analytics, §10.5). Also: **downgrade-to-free** is modeled as `POST /billing/cancel` at period end (there is no `change` to the free tier); the **checkout success** page treats the **webhook as the source of truth** — it polls `billing/summary` until `has_subscription` rather than trusting the Stripe redirect, with a 60s timeout → manual re-check. |
| 019 | Phase 7 §7 org settings wires to the existing org/apikey/audit endpoints; five sitemap↔API gaps are surfaced rather than improvised | The org-settings surface (`/settings/*`) needed no new endpoints, but five mismatches with 02-SITEMAP were surfaced (not silently worked around): **(a)** there is **no org-logo upload endpoint** — only `/me/avatar/upload-url|confirm` exist; `PATCH /orgs/{id}` accepts `logo_key` but nothing mints one — so §7.7.1 general ships **name + slug only** and shows a "logo upload not available yet" note (the `logo_key` field passes through only if a value ever exists). **(b)** `POST /orgs/{id}/invitations` takes a **single** `{email,role}` and is entitlement-gated (`RequireMembers()` → **402 plan_limit_exceeded** at the seat cap), so the multi-email invite form **fans out one POST per address client-side**, surfacing per-email outcomes (sent / 409 already / 422 invalid) and halting on the first 402 with a Billing CTA — there is no batch invite endpoint. **(c)** The member role dropdown offers **ADMIN/MEMBER/GUEST only**; OWNER is assigned solely via `POST /transfer-ownership` (re-gated OWNER in the handler), so the owner's own row and the caller's own row are locked (no self-demote/self-remove — that's Leave/Danger). **(d)** The audit CSV (`GET /audit.csv`) is downloaded via an **authed `apiFetch`→Blob** rather than a plain `<a download>`, because the access token lives in memory (never a cookie/header a bare link could send). **(e)** The settings shell is **UI-gated to ADMIN+** with OWNER-only controls (slug change, ownership transfer, org delete) further gated inside; label writes are MEMBER+ server-side but their UI lives in this admin area (MEMBER label authoring stays deferred). Slug change re-uses the existing `isSlugAvailable` live check and, on success, `router.replace`s to the new `/app/{slug}/settings` + invalidates `['orgs']`; org delete is soft-delete (invariant #7) → the shell's existing grace CTA (ADR-016) then offers restore. |
| 018 | Phase 7 §6 task detail + search/trash/my-tasks composes over the flat `/tasks/{taskId}` API; three sitemap↔API gaps are surfaced rather than improvised | The task surface needed no new endpoints, but three mismatches with 02-SITEMAP were surfaced (not worked around silently): **(a)** task detail URLs are `/projects/{projectKey}/tasks/{taskNumber}`, but tasks are addressed by UUID `{taskId}` only and `tasks/search` carries **no `number` filter** — so number→id resolves through the **board projection** (`GET .../board` lists every card's `number`+`id`), the same cache the board page already holds (`lib/board/use-task.ts`); a task absent from the board (e.g. trashed) 404s the detail page, which is correct. **(b)** The sitemap's task detail is an *intercepted modal over the board with a full page on direct load*; this ships the **full page for both** (the card links there) and **defers the modal interception** — a Next parallel/intercepting route over the client board adds fragility for little gain now, and the full page is the source of truth either way. **(c)** `/trash` and `/my-tasks` are **org-level** in the sitemap, but the API exposes trash **per project** (`GET .../projects/{id}/trash`) and no `assignee=me` alias — so the org trash view **fans out ListTrash across every project and flattens**, and my-tasks passes the caller's own id to search (per ADR-016). There is **no hard-purge endpoint** (trash auto-purges via the retention job), so `/trash` is **restore-only** with explanatory copy, not a purge action. Also: the `board`/task card DTOs carry only user **ids**, so assignee display, the assignee picker and @mention autocomplete are hydrated from `GET /orgs/{id}/members` (`lib/org/use-members.ts`); @mention suggests **org** members (the project-member set is a subset — project-scoped filtering can layer on later via `/projects/{id}/members`). Comment bodies render as pre-wrapped text (a full markdown renderer is out of scope this section). |
| 017 | Phase 7 §5 projects/kanban composes over the existing project API; three gaps are surfaced rather than improvised, and LexoRank is ported to the client | The board area needed no new endpoints, but three mismatches between 02-SITEMAP and the API were surfaced (not silently worked around): **(a)** projects are addressed by UUID `{projectId}` only — there is **no by-key route** — so the `/projects/{projectKey}` URLs resolve key→UUID client-side via `listProjects().find(p => p.key === key)` (one cached list, 404 UI if absent); **(b)** `CreateProject` seeds a fixed default board (`DefaultColumns`) and takes **no template param**, so the create form drops sitemap 7.5.2's "template" field — new boards start with server defaults and columns are edited in project settings; **(c)** board filters are **client-side** over the loaded board projection and cover **text/priority/assignee only** — the `board` card DTO (`taskCardResp`) carries no label ids, so a label filter is impossible without N extra calls (labels remain usable for the bulk **add-label** action, which needs only ids). **LexoRank client port:** `PATCH /tasks/{id}/position` stores the client-supplied `rank` verbatim and 409s on collision, so the drag-drop mints the key in the browser. `web/lib/board/rank.ts` is a faithful port of `backend/internal/pkg/rank/rank.go` (base-36 `0-9a-z`, `mid="i"`, byte order == sort order) — validated with a monotonicity torture test mirroring `rank_test.go` — guaranteeing a client key sorts identically to Postgres `ORDER BY rank`, so a move needs no server round-trip to place a card. Optimistic cache update on drop; on error (esp. 409) the snapshot is restored and the board invalidated. |
| 016 | Phase 7 §4 org shell wires to existing endpoints; five home/edge features are composed from what the API already exposes rather than new endpoints | The shell (`/app/{orgSlug}`) needs role + soft-delete state + a home feed, but no single endpoint carries them. Chosen composition (surfaced, not improvised): **(a)** org context = `GET /orgs` (only source of the caller's per-org role) **+** `GET /orgs/{id}` (only source of `deleted_at`/`logo_key`), joined client-side by slug; **(b)** org-home "recent activity" reuses the caller's own `GET /orgs/{id}/notifications` — there is no org-wide, member-visible activity feed (the audit log is ADMIN-only, so unsuitable); **(c)** "pinned projects" renders the most-recent active projects from `GET /orgs/{id}/projects` — the schema has no pin/recency concept; **(d)** the soft-delete grace screen shows a restore CTA with qualitative copy but **no live countdown**, because `orgResp` exposes `deleted_at` but not the domain's `purge_after` (recommended follow-up: add `purge_after` to `orgResp` for a real countdown); **(e)** the `past_due` banner fetches `GET /orgs/{id}/billing/summary`, which is `read:billing` = ADMIN+, so it is fetched/rendered only for OWNER/ADMIN (a MEMBER/GUEST would 403) — consistent with 02 §9 ("banner for OWNER/ADMIN"). Also note: assigned-to-me passes the caller's own id to `tasks/search?assignee_id=` since there is no server-side `assignee=me` alias. |
| 024 | Deferred follow-ups batch closes ADR-015/016/018/019/022 gaps: org logo upload, admin DTO casing, notification deep-links, task modal | Four items the Phase-7 ADRs explicitly deferred are now built: **(a)** org logo upload — `POST /orgs/{id}/logo/upload-url` (ADMIN, same presign→PUT→PATCH pattern as avatars) with the key namespaced `org-logos/{orgId}/` and enforced in `UpdateProfile`, closing ADR-019's gap; reads mint a short-lived `logo_url` (avatar-style, best-effort) and the shell switcher renders it. **(b)** admin tenant-detail DTOs now serialize snake_case — json tags on the `admin`/`billing` domain structs per ADR-022's own recommendation, with the web `Admin*` types converted in lockstep (verified: no PascalCase field access remains). **(c)** task notifications deep-link — the row carries only the task UUID, so the client resolves project key + task number through the cached board projections (same composition as ADR-018) and navigates to the pretty URL, marking read. **(d)** task detail intercepts into a modal over the board via a `@modal` parallel slot + `(.)tasks/[taskNumber]` route; direct loads/refresh still serve the full page — the ADR-018 modal deferral is lifted, reusing the same `TaskDetail` component as the source of truth. Rejected: separate modal component (would fork the detail UI), server-side URL resolution (no new endpoints needed — composition suffices). | No FR covers intake forms (01/02/08 predate them); this ADR is the spec per the repo rule for doc-reality conflicts. **Model:** `project_forms(org_id, project_id, name, description, target_column_id, token_hash UNIQUE, created_by, is_active)` — RLS-isolated `[T]` like every tenant table (DDL 0027, RLS 0028). The public token is 256-bit `pkg/token` base64url, SHA-256 at rest (same hygiene as invites/API keys, ADR-007/014); the raw token is returned **once** at create/rotate (API-key-style reveal) because only the hash is stored. `target_column_id` carries **no DB FK** (a RESTRICT FK would break column deletion; CASCADE would silently kill the form) — the submit usecase validates column ∈ project instead, and a deleted column fails the submit with 422 until the owner re-points the form. **Unauthenticated reads:** `app_form_by_token(bytea)` SECURITY DEFINER fn (same sanctioned pattern as `app_invitation_by_token`, ADR-014), narrow projection, EXECUTE-granted to `fluxboard_app` only; full form rows stay RLS-gated. **Submit:** resolves org server-side from the token (never from user input), then runs inside `TenantPool.WithTenant` so RLS still backstops the write; `tasks.created_by` is NOT NULL + FK to users, so anonymous tasks are attributed to the form's `created_by` owner with the submitter's name/email prefixed into the description (no core-table schema change). Submit fires the standard `task.created` automation event + SSE publish via the already-wired projectuc collaborators. **Abuse boundary:** `AuthThrottle.PerIP("form_submit")` on the public POST (same as register/reset), per-form `is_active` kill switch, text-only tasks (no attachment/quota surface). **Rejected:** (a) storing the raw token (leaks on DB read); (b) a separate global forms service (tenant isolation invariant #1); (c) requiring login to submit (defeats intake). |
| 023 | Public intake forms: per-project shareable form (`project_forms`) with an unauthenticated submit path | No FR covers intake forms (01/02/08 predate them); this ADR is the spec per the repo rule for doc-reality conflicts. **Model:** `project_forms(org_id, project_id, name, description, target_column_id, token_hash UNIQUE, created_by, is_active)` — RLS-isolated `[T]` like every tenant table (DDL 0027, RLS 0028). The public token is 256-bit `pkg/token` base64url, SHA-256 at rest (same hygiene as invites/API keys, ADR-007/014); the raw token is returned **once** at create/rotate (API-key-style reveal) because only the hash is stored. `target_column_id` carries **no DB FK** (a RESTRICT FK would break column deletion; CASCADE would silently kill the form) — the submit usecase validates column ∈ project instead, and a deleted column fails the submit with 422 until the owner re-points the form. **Unauthenticated reads:** `app_form_by_token(bytea)` SECURITY DEFINER fn (same sanctioned pattern as `app_invitation_by_token`, ADR-014), narrow projection, EXECUTE-granted to `fluxboard_app` only; full form rows stay RLS-gated. **Submit:** resolves org server-side from the token (never from user input), then runs inside `TenantPool.WithTenant` so RLS still backstops the write; `tasks.created_by` is NOT NULL + FK to users, so anonymous tasks are attributed to the form's `created_by` owner with the submitter's name/email prefixed into the description (no core-table schema change). Submit fires the standard `task.created` automation event + SSE publish via the already-wired projectuc collaborators. **Abuse boundary:** `AuthThrottle.PerIP("form_submit")` on the public POST (same as register/reset), per-form `is_active` kill switch, text-only tasks (no attachment/quota surface). **Rejected:** (a) storing the raw token (leaks on DB read); (b) a separate global forms service (tenant isolation invariant #1); (c) requiring login to submit (defeats intake). |
| 015 | Account self-service (`/me`) built in Phase 7 as a dedicated `useruc` usecase; global notification prefs (08 §3) **not** built | The `docs/08 §3` account endpoints (`GET/PATCH /me`, avatar presign, `DELETE /me`) were specced but never implemented in Phases 1–6; the frontend needs them, so they land now. `useruc` is org-independent (no tenant pool); profile reuses `users.avatar_key` (already in 0002) + the MinIO store; `DELETE /me`'s sole-owner guard runs a cross-org read on the **owner pool** (memberships RLS is non-FORCE, ADR-014), returning `409 sole_owner_of` — so account deletion is disabled when `DATABASE_URL_MIGRATE` is unset, consistent with the Phase-6 admin surfaces. **Rejected: global (org-less) notification prefs** — `notification_prefs` is deliberately per-`(org,user,category)` `[T]` with RLS (0013 rejects the 07 §4 org-less shape for invariant #1), so the account "notifications" page composes the existing per-org prefs endpoints with an org selector rather than a new global store. Email-change (`one_time_tokens.purpose='email_change'` already exists) is deferred to a follow-up; the profile UI shows the control disabled until then. |
| 025 | Governed AI layer (mock-first provider, RLS-scoped ai_runs/ai_risks, audit-logged writes, MCP tools) - full text docs/adr/ADR-025-ai-layer.md, FR-AI-001..008 | Mock-first keeps boot green without keys; raw-pgx repos avoid sqlc regen drift; master switch + metering server-enforced. Rejected: query-param tokens, unbounded agents, sqlc regen this phase. |

<!-- docs: dependency rule domain <- usecase <- interface, infrastructure implements domain ports � enforced by go-arch-lint -->
