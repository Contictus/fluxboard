# Build Tracker — Remaining Phases (4–7)

Persistent, resumable checklist for the rest of the Fluxboard backend + frontend
build. It exists because sessions hit context limits mid-phase; this file plus the
per-phase checklists are the **single source of truth for "where are we"** so any
new session resumes with zero re-derivation.

> Phases 0–3 are DONE (auth, tenancy/RBAC, core domain + 3b attachments/search/
> bulk/trash). This tracker covers Phase 4 onward.

---

## ▶ Current Position

- **Phase:** 7 — Frontend (next up; Phase 6 COMPLETE)
- **Next check:** open PHASE-7 (Next.js app wiring the ~55 sitemap routes to the API)
- **Last verified commit:** `bf6988f` (Phase 6 §9 smoke6 + api-key list fix); §1 `66f6399`, §2 `3770b3a`, §3 `76ba018`, §4 `49943b2`, §5 `b86607d`+`f5052d6`, §6 `550144f`, §7 `b099d0e`, §8 `d259c46`
- **Stack state:** migrations at `0016` (0015 admin DDL: `plans.monthly_price` + `api_keys`/`feature_flags`/`entitlement_overrides`; 0016 RLS on the three [T] tables); MODE=stub. Phase 6 DONE end-to-end (smoke6.ps1 ALL GREEN 2026-07-08):
  platform-admin `/admin` router OUTSIDE tenant mw, gated by `PlatformAdminGuard`
  (platform_role=admin AND totp_enabled; impersonation tokens rejected). Cross-org
  admin reads + API-key by-hash auth run on the OWNER pool (`DATABASE_URL_MIGRATE`,
  bypasses non-FORCE RLS). Org API keys (`fbk_live_` + SHA-256, plaintext once) via
  `HybridAuth` (Bearer key OR session) → scope guard (read/write); rate-limited +
  metered on the shared org chain. Impersonation: short-lived `imp`-claim JWT under
  the admin's SID, synthetic read-only ADMIN role, write-guard 403, start audited with
  both identities. Analytics from Phase-5 rollups (`project_stats_daily`) + usage
  dashboard. OpenAPI 3.1 served at `/api/v1/openapi.json` (+ Swagger UI `/api/docs`,
  non-prod). Observability: `http_request_duration_seconds` histogram,
  `asynq_task_processed_total`, `sse_connections_active` gauge, Grafana dashboards;
  `audit:retention` job on the owner pool (audit_log append-only for the app role).
  `cmd/adminctl grant <email>` bootstraps the first admin. **Bug fixed via e2e:**
  `ProjectRepo.List` 500'd on the non-uuid `apikey:<id>` principal → nil-uuid fallback.

Update these four lines whenever a section closes.

---

## Resume protocol (do this at the start of every session)

1. Read **Current Position** above → note the phase and next-check ID.
2. Open that phase file (see index) and jump to the first unticked `- [ ]`
   at or after the pointer — that is the next task.
3. Do exactly that check. Tick it `- [x]`. Small checks = safe stop points.
4. When a whole **section** closes: run its commit-gate check, then update the
   four Current-Position lines (phase / next check / last commit / stack state).
5. Never begin a phase whose predecessor's **Definition of Done** is unticked.

Ticks live inside the phase files, so progress survives any reset. If in doubt,
`grep -c "\- \[x\]" docs/build/PHASE-*.md` shows completion counts per phase.

---

## Phase index

| Phase | File | Modules (FR) | Gate |
|---|---|---|---|
| 4 — Billing | [PHASE-4-BILLING.md](PHASE-4-BILLING.md) | BILL-001..011 | Stripe checkout→webhook→entitlement e2e green |
| 5 — Realtime + Jobs | [PHASE-5-REALTIME-JOBS.md](PHASE-5-REALTIME-JOBS.md) | NTF-001..004 | SSE stream + notification + outbox e2e green |
| 6 — Admin + Observability | [PHASE-6-ADMIN-OBS.md](PHASE-6-ADMIN-OBS.md) | ADM, AUD-003, API, AN | admin surface + API keys + analytics e2e green |
| 7 — Frontend | [PHASE-7-FRONTEND.md](PHASE-7-FRONTEND.md) | 02-SITEMAP (~55 routes) | all sitemap pages wired to API |

Dependency: **4 → 5 → 6 → 7**, strictly (docs/CLAUDE.md §Build Order). Phase 7
can begin area-by-area once the backing API for that area (its phase) is done.

---

## Conventions (every phase file follows these)

- **Check ID `P.S.N`** = phase . section . item (e.g. `4.3.2`). IDs are stable —
  the Current-Position pointer names an exact box.
- **One check = one atomic unit**: a single migration, one repo's method group,
  one handler + route, one test file. Finish-and-stop friendly.
- Each check line states **what**, the **`FR-XXX-NNN`** it advances, and the
  **file path(s)** to create/edit. Verification checks name the exact command.
- **Backend section shape** (phases 4–6), in dependency order:
  `0 Prereqs · 1 Migrations(+RLS) · 2 Domain · 3 Usecase · 4 Infra ·
   5 HTTP · 6 Wiring · 7 Jobs · 8 Tests · 9 E2E verify · 10 Commit gate`.
  Phase 7 sections are per sitemap area.
- **Commit gate** ends each major section: `go build ./... && go vet ./... &&
  go test ./...` green, then `git commit` (conventional message). One section ≈
  one commit, mirroring the Phase 3 rhythm.
- **[VERIFY]** tags a check that runs code/tests rather than writing it.
- **[DECISION]** tags an open choice to resolve before proceeding.

## Project facts a resuming session needs

- Go module root is **`backend/`** (not repo root). Module
  `github.com/mesutokul/fluxboard/backend`. Go 1.25 toolchain.
- Clean layering: `domain` (stdlib only) ← `usecase` ← `interface`; `domain` ←
  `infrastructure`. Enforced by go-arch-lint in CI.
- Every tenant-owned table carries `org_id`; **all** [T] repos go through
  `postgres.TenantPool.WithTenant(orgID, fn)` (SET LOCAL app.current_tenant → RLS).
  `organizations` has NO RLS (readable pre-context; maintenance jobs list orgs via
  `MaintenanceRepo` on the plain pool).
- sqlc v1.31.1: queries in `internal/infrastructure/postgres/queries/*.sql` are the
  source of truth → `sqlc generate` from `backend/`. Never hand-edit `gen/`.
- Migrations split DDL from RLS (e.g. 0007/0008, 0009/0010, 0015/0016). Next free
  number: **0017**.
- Error → HTTP mapping (single place): `ErrValidation→422`, `ErrConflict→409`,
  `ErrNotFound→404`, `ErrForbidden→403`. Billing adds `402 plan_limit_exceeded`.
- Asynq worker + scheduler already run (`cmd/worker`); add handlers to its mux and
  entries to `jobs.Schedule()`.
- Money = integer minor units, never float. Timestamps = `timestamptz` UTC.

## Local dev / verify cheatsheet (Windows, no `make`)

- **Compose up:** `docker compose -f deploy/docker-compose.yml -f
  deploy/docker-compose.override.yml up -d --build api worker` (postgres/redis/
  minio come as deps). Stripe CLI is opt-in: add `--profile stripe` (needs real
  `STRIPE_SECRET_KEY`).
- **Migrate:** `MSYS_NO_PATHCONV=1 docker run --rm --network fluxboard_default -v
  "$(pwd)/backend/migrations":/migrations migrate/migrate -path=/migrations
  -database "postgres://fluxboard_owner:owner_pw@postgres:5432/fluxboard?sslmode=disable"
  up` (down `N` to roll back).
- **HTTP checks:** `curl` is hook-intercepted → use PowerShell `Invoke-RestMethod`;
  API on host port **8080**.
- **Email verify in e2e:** `docker exec -e PGPASSWORD=owner_pw fluxboard-postgres-1
  psql -U fluxboard_owner -d fluxboard -c "UPDATE users SET email_verified=true
  WHERE email='…';"` (login bakes the verified flag into the JWT, so set it first).
- **MinIO presigned PUT from host** fails (signed host `minio:9000`); do it from a
  one-off container on the compose net: `docker run --rm --network fluxboard_default
  curlimages/curl -X PUT --data-binary "…" "<url>"`.
