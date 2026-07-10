# Smoke / e2e scripts

Manual, dockerized end-to-end smokes for the backend phases. They are **not**
wired into CI (they need the full compose stack up); run them locally against a
running API when validating a phase or a release.

| Script | Phase | Exercises |
|---|---|---|
| `smoke4.ps1` | 4 — Billing | free summary → stub checkout → signed `checkout.session.completed` webhook → entitlement upgrade → replay dedup → `invoice.payment_failed` dunning → invoice mirror → free-plan 402 project cap → 429 rate limit |
| `smoke5.ps1` | 5 — Realtime/Jobs | live SSE `task.*`, Last-Event-ID backlog replay, @mention → in-app + email notification, FLUSHALL → resync on reconnect |
| `smoke6.ps1` | 6 — Admin/Observability | admin TOTP bootstrap + `adminctl grant`, tenant list/detail, impersonation (read ok / write 403 / dual-identity audit), API-key one-time reveal + scoped call + rate-limit header, analytics/usage/openapi |

## Run

Bring the stack up first (see `docs/build/README.md` §Local dev):

```
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.override.yml \
  --env-file ./.env up -d --build api worker
```

Then, from a PowerShell prompt, run the script for the phase you're validating,
e.g. `pwsh ./scripts/smoke/smoke4.ps1`. The API host port and credentials are set
at the top of each script.

> No smoke exists yet for Phases 1–3 (auth/tenancy/core) or the Phase-7 UI — a
> Playwright/UI smoke is a tracked follow-up.
