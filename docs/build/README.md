# Build Tracker — Remaining Phases (4–7)

Persistent, resumable checklist for the rest of the Fluxboard backend + frontend
build. It exists because sessions hit context limits mid-phase; this file plus the
per-phase checklists are the **single source of truth for "where are we"** so any
new session resumes with zero re-derivation.

> Phases 0–3 are DONE (auth, tenancy/RBAC, core domain + 3b attachments/search/
> bulk/trash). This tracker covers Phase 4 onward.

---

## ▶ Current Position

- **Phase:** 6 — Admin + observability (next up; Phase 5 COMPLETE)
- **Next check:** open PHASE-6 (admin panel, audit log viewer, Prometheus/Grafana)
- **Last verified commit:** `9cc6921` (Phase 5 §9 smoke5 + SSE fixes); §4 `14d5560`, §5 `b7c6719`, §6 `20ac75a`, §7 `8eda8bf`, §8 `08dc789`
- **Stack state:** migrations at `0014` (0013 notifications/prefs/stats DDL, 0014 RLS);
  plans seeded; MODE=stub. Phase 5 DONE end-to-end (smoke5.ps1 ALL GREEN 2026-07-07):
  SSE stream `GET /orgs/{org}/events` over Redis Streams (`events:{org}`, XADD MAXLEN
  ~1000; per-org consumer; Last-Event-ID replay; resync on flush/gap), notification
  center (`/notifications` list/unread/read/read-all + `/prefs` matrix), fan-out
  (@mention/assigned/comment/invite → in-app rows + email:send outbox, send-time pref
  recheck), and jobs email:send (shared handler, dispatches billing vs notification)
  + stats:rollup (nightly project_stats_daily). Producers (task/project/tenant/billing)
  publish via notify.EventBus. **Deferred to Phase 6:** org:hard_delete, audit:retention,
  webhook:retry (need privileged/admin infra — see PHASE-5 §7 deferral note). Two SSE
  bugs fixed in §9: logging statusWriter now passes through http.Flusher; Replay returns
  gap on an empty stream. Phase-4 billing jobs + rate limiting still live.

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
- Migrations split DDL from RLS (e.g. 0007/0008, 0009/0010). Next free number:
  **0011**.
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
