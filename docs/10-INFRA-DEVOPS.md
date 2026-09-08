# 10 — Infrastructure, DevOps & Observability

## 1. Docker Compose (local = the reference environment)

`deploy/docker-compose.yml` services:

| Service | Image | Ports | Notes |
|---|---|---|---|
| postgres | postgres:16-alpine | 5432 | volume `pgdata`; init script creates roles `fluxboard_owner/app/admin_ro`; healthcheck `pg_isready` |
| redis | redis:7-alpine | 6379 | `--appendonly yes` |
| minio | minio/minio | 9000/9001 | volume; `minio-init` one-shot creates bucket `fluxboard` + access policy |
| stripe-cli | stripe/stripe-cli | — | `listen --forward-to api:8080/api/v1/webhooks/stripe`; webhook secret injected via env |
| mailpit | axllent/mailpit | 8025 (UI) / 1025 (SMTP) | local email capture |
| api | build backend (target api) | 8080 | depends_on healthy pg/redis/minio; hot reload via `air` in dev override |
| worker | build backend (target worker) | — | shares image with api |
| web | build web / `next dev` | 3000 | |
| prometheus | prom/prometheus | 9090 | scrape api:8080/metrics, worker:8081/metrics |
| grafana | grafana/grafana | 3001 | provisioned datasource + dashboards from `deploy/grafana/` |
| asynqmon | hibiken/asynqmon | 8082 | queue introspection UI |

Backend Dockerfile: multi-stage — `golang:1.25` build (CGO_ENABLED=0,
`-ldflags "-s -w -X main.version=$GIT_SHA"`) → `gcr.io/distroless/static`
runtime, nonroot user. Two targets (api/worker) from one build stage.
`docker-compose.override.yml` mounts source + runs `air` for dev; base file
is prod-shaped.

## 2. Configuration

12-factor: env only, parsed once at startup into a `Config` struct
(`envconfig`), fail-fast on missing required keys. `.env.example` committed;
`.env` gitignored. Key vars: `DATABASE_URL` (app role), `DATABASE_URL_MIGRATE`
(owner role), `REDIS_ADDR`, `JWT_PRIVATE_KEY_PEM` (or path), `STRIPE_SECRET_KEY`,
`STRIPE_WEBHOOK_SECRET`, `MINIO_*`, `SMTP_*`, `WEB_ORIGIN`, `TOTP_ENC_KEY`.
Secrets never logged; config struct implements a redacting `String()`.

## 3. Makefile Targets

```
make up                        start the full stack and recreate images
make down                      stop the stack and remove its volumes
make logs                      tail all service logs
make migrate / migrate-down    golang-migrate against DATABASE_URL_MIGRATE
make sqlc                      regenerate query code
make gen-client                print the current client-generation guidance
make seed                      print the not-yet-implemented seed notice
make stripe-seed                print the not-yet-implemented Stripe seed notice
make api / worker / web        dev processes
make test / test-integration   Go unit / integration-tag suites
make lint                      golangci-lint + go-arch-lint
make audit                     govulncheck (backend only)
```

## 4. CI (GitHub Actions — `.github/workflows/ci.yml`)

Jobs (parallel where independent):

1. **lint:** golangci-lint, go-arch-lint (dependency rule 03 §3), and `sqlc diff`
   (generated code current).
2. **test-backend:** unit and integration-tag tests with PostgreSQL, Redis, and
   MinIO service containers.
3. **build:** Docker-build both backend images with the commit SHA tag.
4. **web:** frozen pnpm install, TypeScript check, ESLint, Vitest, and Next.js
   production build.
5. **security:** govulncheck and gitleaks. npm audit and Playwright smoke are
   not currently workflow steps; the dockerized UI smoke remains a deferred
   follow-up tracked in `docs/build/README.md`.

Branch protection: PRs to main require jobs 1–3 green.

## 5. Observability

**Logging:** slog JSON to stdout. Base fields per request: request_id,
user_id, org_id, method, path, status, duration_ms. Levels: ERROR pages a
human (in principle), WARN is actionable, INFO is request/lifecycle, DEBUG
off outside dev. No PII beyond IDs; never token material, never card data
(none exists), email only at INFO for auth events.

**Metrics (Prometheus):**

```
http_request_duration_seconds{route,method,status}   histogram
http_requests_in_flight                              gauge
auth_login_total{result}                             counter
auth_refresh_reuse_total                             counter  ← security signal
webhook_events_total{type,outcome}                   counter
webhook_processing_duration_seconds                  histogram
billing_reconciliation_drift_total                   counter
asynq_task_processed_total{type,status} + queue depth (asynq exporter)
sse_connections_active{}                             gauge
usage_flush_duration_seconds                         histogram
pgxpool stats (acquired/idle), go runtime defaults
```

**Grafana dashboards (provisioned JSON in repo — portfolio artifact):**
1. *API Overview* — RPS, p50/p95/p99 by route, error rate, in-flight
2. *Billing Health* — webhook outcomes, processing latency, drift, dunning funnel
3. *Jobs & Queues* — Asynq depth by queue, failure rate, dead-letter count
4. *Security* — login failures, refresh-reuse events, 403/404-probe rates, rate-limit hits

**Endpoint exposure:** `/metrics` (api :8080, worker :8081) is scraped over the
internal compose/cluster network only — it is NOT routed through the public
ingress/LB. Locally, Prometheus reaches it by compose-service DNS
(`api:8080`, `worker:8081`); in prod, keep the metrics path behind a network
policy / firewall (no auth is applied at the handler). `/readyz` gates traffic on
DB + Redis (+ optional MinIO/Stripe); `/healthz` is liveness only. (6.7.5)

**Alert rules (prometheus rules file, documented even if only local):**
webhook error-rate > 5%/10 m, dead-letter > 0, readyz failing 3×,
refresh_reuse > 0 (info-level security alert), queue depth > 1000.

## 6. Operational Behaviors

- **Graceful shutdown:** SIGTERM → stop accepting, drain in-flight (20 s),
  close SSE with `event: shutdown` (clients reconnect to another replica),
  Asynq stops claiming and finishes current tasks.
- **Readiness vs liveness:** `/readyz` gates traffic (DB+Redis+migration
  version check); `/healthz` only process-up — restart loops never caused by
  dependency blips.
- **Migrations:** run as a separate step/job (never on api boot in prod
  shape); `migrate-down` tested in CI for the last 3 migrations.
- **Backups (documented posture):** `pg_dump` nightly in compose via sidecar
  to MinIO bucket `backups/` + restore runbook in `docs/runbooks/restore.md`
  (portfolio artifact: an actually-tested restore, with the transcript).

## 7. Production-Shape Notes (documented, not required to deploy)

Single-region deployment story for the README: containers behind a reverse
proxy (Caddy/nginx) terminating TLS; Postgres managed (RDS-class) with PITR;
Redis managed; MinIO → S3; secrets via SSM/Vault; horizontal scale = N api
replicas (stateless: sessions in PG/Redis, SSE via Redis Streams) + M workers.
Explicitly listed single-points and their mitigation order — this section
exists to answer the interview question "how would this run in production".
