# Fluxboard

Multi-tenant project management + usage-based billing SaaS — a production-grade
portfolio backend (Linear/Jira core + Stripe metered billing). Go modular
monolith (api + worker), PostgreSQL with RLS, Redis, MinIO, Stripe.

Full spec lives in [`docs/`](docs/); start with
[`docs/CLAUDE.md`](docs/CLAUDE.md) and read in the order listed there.

## Status

**Phase 0 — Scaffolding** (complete). The stack stands up healthy with no
business logic yet: repo layout, Docker Compose reference environment,
migrations tooling, build tooling, and CI. Build order and per-phase scope are
in `docs/CLAUDE.md` §Build Order.

## Tech stack (fixed)

Go 1.25 · chi · sqlc + pgx/v5 · golang-migrate · PostgreSQL 16 (RLS) ·
Redis 7 · Asynq · MinIO · Stripe · Prometheus + Grafana · slog (JSON).

> Note: the docs pin "Go 1.22+"; module dependencies require the 1.25 toolchain,
> which satisfies that floor. Docker images and CI use 1.25.

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
deploy/      docker-compose (+dev override), postgres init, prometheus, grafana
docs/        full specification (source of truth)
.github/     CI workflow
```

Architecture and the enforced dependency rule (`domain ← usecase ← interface`,
`domain ← infrastructure`) are described in `docs/03-ARCHITECTURE.md` and
checked by `go-arch-lint` in CI.
