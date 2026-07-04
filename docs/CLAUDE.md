# CLAUDE.md — Project Instructions for Claude Code

## What This Project Is

**Fluxboard** — a multi-tenant project management + usage-based billing SaaS platform.
Think: Linear/Jira core + Stripe metered billing. Built as a production-grade portfolio
project demonstrating senior-level backend engineering.

## Documentation Map (READ IN THIS ORDER)

| File | Purpose |
|------|---------|
| `00-PROJECT-OVERVIEW.md` | Vision, scope, tech stack, non-goals |
| `01-FUNCTIONAL-REQUIREMENTS.md` | Exhaustive FR list with stable IDs (FR-XXX-NNN) |
| `02-SITEMAP.md` | Every page URL, route guards, layout hierarchy |
| `03-ARCHITECTURE.md` | System architecture, folder structure, ADRs |
| `04-AUTH.md` | JWT + refresh rotation + OAuth2 PKCE spec |
| `05-TENANCY-RBAC.md` | Multi-tenancy isolation (PostgreSQL RLS) + Casbin RBAC/ABAC |
| `06-BILLING.md` | Stripe integration, webhook idempotency, usage metering |
| `07-DATABASE-SCHEMA.md` | Complete DDL, indexes, RLS policies, migrations |
| `08-API-SPEC.md` | REST API endpoints, error contract, pagination |
| `09-REALTIME-JOBS.md` | WebSocket events, Asynq background jobs, notifications |
| `10-INFRA-DEVOPS.md` | Docker Compose, CI/CD, observability stack |
| `11-SECURITY.md` | Threat model, security checklist, OWASP mapping |
| `12-TESTING.md` | Test strategy, coverage targets, test data |

## Tech Stack (FIXED — do not substitute)

- **Backend:** Go 1.22+, chi router, sqlc (NOT GORM), pgx/v5, golang-migrate
- **Auth:** Custom JWT (golang-jwt/jwt/v5), Argon2id (alexedwards/argon2id), OAuth2 (golang.org/x/oauth2), Casbin v2
- **Database:** PostgreSQL 16 with Row-Level Security
- **Cache/Queue:** Redis 7 (go-redis/v9), Asynq for background jobs
- **Payments:** Stripe (stripe-go/v78), Stripe CLI for local webhook forwarding
- **Object storage:** MinIO (S3-compatible), presigned URLs
- **Frontend:** Next.js 14 App Router, TypeScript strict, TanStack Query v5, Tailwind, shadcn/ui, dnd-kit (kanban), Recharts
- **Observability:** Prometheus + Grafana, structured logging with slog (JSON)
- **Local dev:** Docker Compose (single `make up` brings everything up)

## Build Order (STRICT — each phase depends on the previous)

1. **Phase 0 — Scaffolding:** repo layout, Docker Compose (postgres/redis/minio), migrations tooling, Makefile, CI skeleton
2. **Phase 1 — Auth:** register/login/refresh rotation/logout, Argon2id, session table, OAuth2 Google (PKCE), email verification tokens
3. **Phase 2 — Tenancy + RBAC:** organizations, memberships, invitations, RLS policies, Casbin model + middleware
4. **Phase 3 — Core domain:** projects, boards, tasks, subtasks, labels, comments, attachments (MinIO presigned)
5. **Phase 4 — Billing:** Stripe customers/subscriptions/checkout, webhook consumer with idempotency, usage metering pipeline, plan enforcement middleware
6. **Phase 5 — Realtime + jobs:** SSE event stream, Asynq workers (email, usage aggregation, webhook retry), notification center
7. **Phase 6 — Admin + observability:** platform admin panel, audit log viewer, Prometheus metrics, Grafana dashboards
8. **Phase 7 — Frontend completion:** all pages in 02-SITEMAP.md wired to API

Do NOT start a phase before the previous phase's acceptance criteria
(defined in 01-FUNCTIONAL-REQUIREMENTS.md) pass.

## Code Conventions

### Go
- Clean Architecture: `domain` has zero external imports; `usecase` imports domain only;
  `infrastructure` implements repository interfaces defined in domain.
- Every repository method takes `context.Context` as first parameter.
- Every DB query MUST run through the tenant-scoped connection wrapper
  (sets `app.current_tenant` for RLS — see 05-TENANCY-RBAC.md §4). Direct pool
  access outside the wrapper is a defect, not a style issue.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; sentinel errors in domain package
  (`domain.ErrNotFound`, `domain.ErrForbidden`, `domain.ErrConflict`); HTTP layer maps
  sentinels to status codes in ONE place (`internal/interface/http/errmap.go`).
- No global state except the DI container built in `cmd/api/main.go`.
- sqlc: all queries live in `internal/infrastructure/postgres/queries/*.sql`.
  Regenerate with `make sqlc`. Never hand-edit generated code.

### TypeScript / Next.js
- Strict mode on. No `any`. API types generated from OpenAPI spec (`make gen-client`).
- Server Components by default; `"use client"` only where interactivity requires it.
- All mutations through TanStack Query mutations with optimistic updates
  where specified in 01-FUNCTIONAL-REQUIREMENTS.md.
- Auth: access token in memory (React context), refresh token in httpOnly
  Secure SameSite=Strict cookie. NEVER localStorage for tokens.

### Git
- Conventional commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- One phase = one or more feature branches merged to `main` via PR.

## Non-Negotiable Invariants

1. **Tenant isolation:** no query may return rows from another tenant. RLS is the
   backstop; the application layer must ALSO filter. Defense in depth.
2. **Webhook idempotency:** every Stripe event processed at most once
   (processed_events table, unique on event_id, same transaction as side effects).
3. **Refresh token rotation:** reuse of a rotated token revokes the entire token family.
4. **Card data never touches our servers:** Stripe Checkout / Payment Element only.
5. **All money amounts:** integer minor units (cents/kuruş). NEVER float.
6. **All timestamps:** `timestamptz`, UTC in DB, localized only at render.
7. **Soft delete for billing-relevant entities** (organizations, subscriptions);
   hard delete allowed only for tasks/comments per data retention rules.

## When Reality Conflicts With These Docs

Stop and surface the conflict. Do not silently improvise. Propose the change
as an ADR entry in `03-ARCHITECTURE.md` §ADR before implementing.

## Local Development Quickstart

```bash
make up          # docker compose: postgres, redis, minio, stripe-cli, mailpit
make migrate     # run all migrations
make seed        # seed demo tenant + users (see 12-TESTING.md §5)
make api         # run Go API with hot reload (air)
make web         # run Next.js dev server
make test        # unit + integration tests (testcontainers)
```
