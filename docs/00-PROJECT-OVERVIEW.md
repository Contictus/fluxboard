# 00 — Project Overview

## 1. Product Definition

**Fluxboard** is a multi-tenant B2B SaaS for project and task management with
usage-based billing. Organizations (tenants) sign up, invite members, manage
projects on kanban boards, and are billed monthly through Stripe based on
their plan tier plus metered usage (active members, storage, API calls).

The product is a **portfolio project**: every module is chosen to demonstrate
a specific production engineering competency, not to maximize feature count.

| Module | Competency Demonstrated |
|--------|------------------------|
| Auth (custom JWT + OAuth2 PKCE) | Session security, token lifecycle, standards (RFC 6749/7636/9068) |
| Multi-tenancy (PostgreSQL RLS) | Data isolation, defense in depth |
| RBAC/ABAC (Casbin) | Authorization beyond simple role checks |
| Stripe billing | Idempotency, webhook reliability, financial correctness |
| Usage metering | Redis→Postgres aggregation pipeline, eventual consistency |
| Realtime (SSE) + Asynq jobs | Async architecture, at-least-once processing |
| Audit log + admin panel | Compliance thinking, operational tooling |
| Observability (Prometheus/Grafana) | Production operations mindset |

## 2. Actors

| Actor | Description |
|-------|-------------|
| **Visitor** | Unauthenticated user on public pages |
| **User** | Authenticated person; may belong to 0..N organizations |
| **Org roles** | OWNER > ADMIN > MEMBER > GUEST (per-organization membership role) |
| **Project roles** | project-level assignment overlaying org role (LEAD, CONTRIBUTOR, VIEWER) |
| **Platform Admin** | Fluxboard staff; cross-tenant access via dedicated admin surface, fully audited |

## 3. Plan Tiers

| | Free | Pro | Business |
|---|---|---|---|
| Price (base, monthly) | $0 | $12/seat | $24/seat |
| Members | 3 | 25 | unlimited (metered) |
| Projects | 2 | 20 | unlimited |
| Storage | 100 MB | 10 GB | 100 GB + $0.10/GB overage |
| API rate limit | 60 req/min | 600 req/min | 3000 req/min |
| Metered dimensions | — | — | seats above 25, storage overage, API calls above 1M/mo |
| Audit log retention | 7 days | 90 days | 2 years |
| SSO (Google OAuth) | ✓ | ✓ | ✓ |
| Priority webhook events | — | — | ✓ |

Plan limits are enforced server-side by the **entitlement middleware**
(06-BILLING.md §7) — never only in the UI.

## 4. Tech Stack Summary

Backend Go 1.25 (chi, sqlc, pgx, Casbin, Asynq, stripe-go), PostgreSQL 16
(RLS), Redis 7, MinIO, Next.js 14 App Router + TypeScript strict + TanStack
Query + Tailwind/shadcn, Prometheus + Grafana, Docker Compose. Full rationale
and rejected alternatives in 03-ARCHITECTURE.md §ADR.

## 5. Explicit Non-Goals (v1)

- Native mobile apps (responsive web only)
- Self-hosted / on-prem distribution
- SAML / SCIM enterprise SSO (OIDC via Google only in v1)
- Gantt charts, time tracking, sprints/velocity beyond the basic analytics page
- Multi-region deployment; single-region with documented DR posture
- Payment providers other than Stripe; multi-currency (USD only in v1)
- Public plugin/app marketplace

## 6. Quality Targets

| Dimension | Target |
|---|---|
| API p95 latency (CRUD) | < 150 ms local, < 300 ms under seeded load |
| Test coverage | usecase layer ≥ 85%, billing package ≥ 95% |
| Webhook processing | zero duplicate side effects under replay test (12-TESTING.md §7) |
| Tenant isolation | cross-tenant access test suite must pass 100% (12-TESTING.md §6) |
| Cold start (compose) | `make up && make migrate && make seed` < 3 min |
| Lighthouse (marketing pages) | ≥ 90 performance/accessibility |

## 7. Repository Layout (top level)

```
fluxboard/
├── CLAUDE.md
├── docs/                    # these design documents
├── backend/                 # Go API + worker (03-ARCHITECTURE.md §3)
├── web/                     # Next.js app
├── deploy/
│   ├── docker-compose.yml
│   ├── grafana/             # provisioned dashboards
│   └── prometheus/
├── Makefile
└── .github/workflows/       # ci.yml, described in 10-INFRA-DEVOPS.md
```
