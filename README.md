# Fluxboard

Multi-tenant project management + usage-based billing SaaS — a production-grade
portfolio backend (Linear/Jira core + Stripe metered billing). Go modular
monolith (api + worker), PostgreSQL with RLS, Redis, MinIO, Stripe.

Full spec lives in [`docs/`](docs/); start with
[`docs/CLAUDE.md`](docs/CLAUDE.md) and read in the order listed there.

## Status

**All build phases (0–7) complete.**

- **0–2** — scaffolding, auth (JWT + refresh rotation, OAuth2 PKCE, email
  verification), tenancy + RBAC/RLS + audit.
- **3** — project-management core: projects, membership/roles, boards + columns
  (LexoRank ordering), tasks with per-project numbering, subtasks, org labels,
  comments (15-minute edit window), activity log, MinIO attachments (presigned
  PUT/GET, 25 MiB + MIME allowlist, nightly orphan GC), PostgreSQL full-text
  search (`tsvector` + GIN, faceted + paginated), single-transaction bulk board
  actions, and soft-delete → Trash → restore with a nightly 30-day purge job.
- **4 — Billing** — Stripe customers/subscriptions/checkout, webhook consumer
  with idempotency, Redis→Postgres usage metering, plan-enforcement middleware.
- **5 — Realtime + jobs** — SSE event stream, Asynq workers (email, usage
  aggregation, stats rollup), notification center.
- **6 — Admin + observability** — platform-admin surface, audit-log viewer, org
  API keys, project analytics, Prometheus + Grafana.
- **7 — Frontend** — every page in `docs/02-SITEMAP.md` wired to the API
  (Next.js 14 App Router, TypeScript strict, 55 `page.tsx` route files).

Per-phase scope and the resumable build checklist are in
[`docs/build/`](docs/build/). Deferred, non-blocking follow-ups: dockerized
end-to-end smoke of the new UI, org-logo upload endpoint (ADR-019), snake_case
JSON tags on the admin tenant-detail DTOs (ADR-022), task-notification
deep-linking (ADR-018).

## Tech stack (fixed)

Go 1.25 · chi · sqlc + pgx/v5 · golang-migrate · PostgreSQL 16 (RLS) ·
Redis 7 · Asynq · MinIO · Stripe · Prometheus + Grafana · slog (JSON) ·
Next.js 14 (App Router, TypeScript strict) · TanStack Query · Tailwind + shadcn/ui.

## Quickstart

```bash
cp .env.example .env      # local dev defaults; edit secrets as needed
make up                   # docker compose: postgres, redis, minio, mailpit,
                          #   prometheus, grafana, asynqmon, api, worker
make migrate              # apply migrations (owner role)
make api                  # (or run in-stack) API with hot reload via air
make test                 # go unit tests
```

Then:

- API health: <http://localhost:8080/healthz> · <http://localhost:8080/readyz>
- API metrics: <http://localhost:8080/metrics>
- Mailpit: <http://localhost:8025> · MinIO console: <http://localhost:9001>
- Prometheus: <http://localhost:9090> · Grafana: <http://localhost:3001>
- Asynq queues: <http://localhost:8082>

Run `make help` for all targets.

## Layout

```
backend/     Go module: cmd/{api,worker}, internal/{domain,usecase,infrastructure,interface,pkg}
web/         Next.js 14 App Router frontend (pnpm)
deploy/      docker-compose (+dev override), postgres init, prometheus, grafana
docs/        full specification (source of truth)
scripts/     smoke tests + tooling
.github/     CI workflow
```

Architecture and the enforced dependency rule (`domain ← usecase ← interface`,
`domain ← infrastructure`) are described in `docs/03-ARCHITECTURE.md` and
checked by `go-arch-lint` in CI.
