# 09 — Realtime (SSE) & Background Jobs (Asynq)

## 1. SSE Event Stream (FR-NTF-001)

**Endpoint:** `GET /api/v1/orgs/{orgId}/events` — standard middleware chain
applies (auth + membership). Response: `text/event-stream`,
`Cache-Control: no-cache`, `X-Accel-Buffering: no`, heartbeat comment `:ka`
every 25 s (proxy idle-timeout defense).

**Fan-out architecture:** producers (usecases) publish to a **Redis Stream**
per org — `events:{orgId}` (XADD, MAXLEN ~1000, ≈5 min of history under
normal load). Each API instance runs one consumer goroutine per org with
active subscribers (XREAD BLOCK), demuxing to in-process subscriber channels.
This makes SSE horizontally scalable: any API replica can serve any
subscriber; the stream is the bus.

**Event wire format:**

```
id: 1720000000000-0            ← Redis stream entry ID (doubles as Last-Event-ID)
event: task.moved
data: {"task_id":"…","project_key":"PAY","from_column":"…","to_column":"…",
       "rank":"…","actor_id":"…","v":1}
```

**Replay:** client reconnects with `Last-Event-ID` header → server XRANGE
from that ID. ID older than stream retention → server sends
`event: resync` → client invalidates TanStack Query caches for board/task
keys and refetches. This bounds worst-case staleness without an unbounded log.

**Event catalog:** `task.created|updated|moved|deleted|restored`,
`comment.created`, `member.joined|left|role_changed`, `membership.revoked`
(targeted — client for that user hard-redirects out of org),
`notification.created` (targeted by user_id filter client-side),
`billing.status_changed`, `resync`.

**Client (`web/lib/sse.ts`):** EventSource wrapper; reconnect backoff
1s→2s→5s→10s cap with jitter; per-event handlers update TanStack Query cache
surgically (e.g. `task.moved` → move card in `['board',org,project]` cache)
instead of blanket invalidation; actor's own events skipped by
`actor_id === me` (optimistic update already applied).

## 2. Background Jobs (Asynq)

Queues and priorities: `critical` (6): billing sync, outbox drain ·
`default` (3): emails, notifications fan-out · `low` (1): rollups, GC,
purges. Worker: `cmd/worker`, concurrency 10, Prometheus metrics exported.

| Task type | Trigger | Idempotency mechanism |
|---|---|---|
| `outbox:drain` | every 5 s (scheduler) | rows claimed via `UPDATE … WHERE drained_at IS NULL RETURNING`; enqueued Asynq task carries outbox id as Asynq TaskID (dedup) |
| `email:send` | outbox | Asynq TaskID = outbox row id → at-most-once enqueue; SMTP send retried ×5 exp backoff |
| `usage:aggregate` | hourly cron | UPSERT ON CONFLICT (06 §5) — re-runnable |
| `usage:push_stripe` | daily 02:00 UTC | Stripe `action=set` (absolute) — re-runnable |
| `stats:rollup` | nightly 03:00 | recomputes yesterday's `project_stats_daily` rows (UPSERT) |
| `gc:orphan_uploads` | nightly | deletes MinIO objects for `attachments.status='pending'` older than 24 h, then rows |
| `gc:trash_purge` | nightly | hard-deletes tasks with `deleted_at < now()-30d` (+ MinIO objects) |
| `org:hard_delete` | scheduled at soft-delete +14 d | guarded by `purge_after <= now()` re-check; cancels if restored |
| `audit:retention` | nightly | per-org delete older than plan retention (privileged role — app role lacks DELETE) |
| `billing:reconcile` | nightly 04:00 | 06 §8; pure read+heal, naturally idempotent |
| `webhook:retry` | admin button | re-invokes consumer → dedup via processed_stripe_events (safe no-op if handled) |

**Handler contract:** every handler idempotent (stated mechanism above),
context-aware (respects cancellation), max 3 retries then dead-letter
(`asynq:dead`) — surfaced in `/admin/jobs`. Failure of one org's job never
aborts the batch (per-org error isolation, errors aggregated to log).

**Transactional outbox rationale (ADR-011 companion):** enqueueing Asynq
directly inside a DB transaction is not atomic (Redis enqueue can succeed
while the tx rolls back, or vice versa). Writes that must trigger jobs insert
into `outbox` within the same tx; the drainer provides at-least-once handoff;
Asynq TaskID dedup + idempotent handlers close the loop to exactly-once
effect.

## 3. Notification Fan-out (FR-NTF-002/003)

Producer path (e.g. comment with @mention):

```
taskuc.AddComment tx:
  insert comment → insert task_activity → resolve mention targets
  → insert notifications rows → insert outbox(email:send per opted-in target)
after commit:
  publish comment.created + notification.created to events:{orgId}
```

Preference check at SEND time (worker re-reads `notification_prefs`) —
a user disabling emails mid-queue is respected. Transactional auth emails
(verify, reset) bypass preferences by category whitelist.

## 4. Failure Scenarios To Verify (12 §8)

- Kill worker mid `usage:push_stripe` → rerun → Stripe quantities unchanged (set-semantics)
- Redis flush → SSE clients receive `resync` on reconnect; board state self-heals from API
- 500 subscriber connections on one org (load test): stream fan-out CPU + memory profile recorded in README
- Outbox drainer down 10 min → backlog drains completely, no duplicate emails (TaskID dedup)
