# ADR-025 — Governed AI Layer (mock-first, tenant-scoped, audit-logged)

Date: 2026-09-18. Status: accepted. Implements FR-AI-001..008.

## Context

2026 rakip standardı: ClickUp Brain Super Agents, Asana AI Studio, Monday
Sidekick, Wrike risk prediction, Jira Rovo. Fluxboard sıfır AI. En büyük fark.
Altyapı hazır: RLS tenant izolasyon (05 §4), append-only audit_log (0005),
entitlement 402 middleware (06 §7), Asynq jobs, SSE stream.

## Decision

AI katmanı 4 dilim, mock-first:

1. **NL parse + plan draft** — `POST /orgs/{id}/ai/parse`, `POST
   /orgs/{id}/ai/plan-draft`. Plan atomic: hepsi ya da hiçbiri. Idempotency-Key
   zorunlu (checkout/invite ile aynı Redis store, 24s).
2. **Status digest** — canlı proje verisinden sponsor-ready rapor. Sayı uydurma
   yok: engine hesaplar, AI prose anlatır (EVM/SPI-CPI dahil).
3. **Risk signals** — overdue, overcommit, dependency churn, scope değişim
   sıklığı. Persist + dismissable, aynı alert tekrar noise yapmaz. BUSINESS+
   gate (mevcut entitlement middleware reuse).
4. **Chat + MCP** — 15 typed tool (task list/create/move, digest). MCP
   `POST /api/v1/mcp`, HybridAuth (session veya `fbk_live_`), scope guard aynı.

## Governance (non-negotiable)

- **Tenant scope:** prompt context org-scoped only. Cross-tenant probe → 404,
  aynı kural (invariant #1). RLS backstop: `ai_runs`/`ai_risks` `[T]`,
  `TenantPool` + raw-tx helper (sqlc regen yok — audit_repo.go precedent:
  hand-written pgx).
- **Audit:** her AI write `audit_log severity=ai`, metadata model+tokens+kind.
  Append-only, app rolü UPDATE/DELETE yok (0005).
- **Master switch:** org-level AI kill switch + surface toggle, server-side
  enforce. Kapalıysa direkt çağrı da 403.
- **Metering:** token/aksiyon sayacı mevcut usage pipeline (FR-BILL-007) reuse.
  Aylık cap aşımı → 402 `plan_limit_exceeded` limit=`ai_actions`.
- **Retention:** AI input/output 30d, purge job (audit retention ile aynı pattern,
  owner pool).
- **Provider:** interface + mock (deterministic, testler). Canlı Claude/Azure
  OpenAI failover ayrı commit, key yoksa mock — boot asla fail etmez (MinIO
  patterni: nil ⇒ disabled değil, mock ⇒ safe).

## Rejected

- Query-param token ile SSE/AI stream (credential leak, ADR-021).
- Ham token/secret loglama (String() redaction, config §2).
- sqlc regen bu fazda (CI pin v1.27.0 vs local v1.31.1 drift riski; raw pgx
  yeterli, sqlc port follow-up).
- Sınırsız otonom agent (write blast radius; tool allowlist + impersonation
  read-only kuralı geçerli).

## Consequences

- Migration 0030 DDL + 0031 RLS (`ai_runs`, `ai_risks`).
- `domain/ai`, `usecase/aiuc`, `infrastructure/ai` (provider), postgres
  `ai_repo.go` (raw pgx), `handlers/ai.go` + `handlers/mcp.go`, jobs
  `ai_digest` + `ai:retention`, web chat panel.
- 0030-0032 schema migrationlari `gen/models.go`'ya AiRun/AiRisk ekler,
  `sqlc generate` (pin v1.27.0) ile regen edilir, CI `sqlc diff` dogrular.

## Addendum 2026-09-18 — plan apply (FR-AI-009)

Draft → tasks writes through `taskuc.CreateTask` per item (per-project
numbering, LexoRank append, SSE events, automation, assignment notifications
all inherited — no parallel write path). Atomicity is validate-then-create
with compensation: cheap rules (count ≤ 50, title 1..200, priority valid,
date range) pre-validate before the first write; authoritative gates
(CONTRIBUTOR+, column ∈ project) run inside taskuc per item; on the first
failure already-created tasks are trashed (best-effort, logged) so the board
never holds partial state — residue lands in Trash (restorable, 30d purge),
never on the board. `Idempotency-Key` required (Redis + `plan_apply` run-row
UNIQUE backstop, same pattern as plan draft). Caller role flows from
`TenantContext.Role` (sessions, keys, impersonation-readonly all reuse the
standard chain). Rejected: single-transaction bulk insert (repos own their
tx scope; sharing tx across repos breaks the TenantPool discipline), hard
purge compensation (trash is the designed undo area).
