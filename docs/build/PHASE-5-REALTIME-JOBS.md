# Phase 5 — Realtime (SSE) + Background Jobs + Notifications

**Modules:** NTF-001..004 · **Spec:** `docs/09-REALTIME-JOBS.md`, `docs/01-…md` §NTF.
**Depends on:** Phase 4 (outbox table + worker deps). **Gate:** SSE stream +
notification fan-out + outbox e2e green.

> Builds the realtime bus and the async backbone. The transactional `outbox`
> (created in Phase 4) is the atomicity primitive: DB writes that must trigger
> jobs insert into `outbox` in the same tx; the drainer hands off to Asynq;
> TaskID dedup + idempotent handlers = exactly-once effect.

---

## Section 0 — Prereqs & decisions

- [x] 5.0.1 [VERIFY] Confirm Phase-4 `outbox` table + `outbox:drain` job exist (dependency). If billing was stubbed, ensure outbox landed regardless. — `outbox` [T] in `0011_billing.up.sql` (partial index `outbox_undrained_idx WHERE drained_at IS NULL`); `outbox:drain` handler + `@every 5s` schedule live in `internal/interface/jobs/billing.go` (TaskID=`outbox:{id}` dedup, queue `critical`). Landed regardless of stub.
- [x] 5.0.2 [DECISION] Confirm fan-out = **Redis Stream per org** (`events:{orgId}`, XADD MAXLEN ~1000), per 09 §1 — records the horizontal-scale rationale. **CONFIRMED**: Redis Stream per org. Rationale (09 §1): stream is the bus, so any API replica serves any subscriber (horizontal scale); ~1000 MAXLEN ≈ 5 min history bounds replay window; entry ID doubles as `Last-Event-ID`. No Postgres LISTEN/NOTIFY (doesn't survive multi-replica + no replay).
- [x] 5.0.3 Freeze the event catalog + wire format in this file (see **Frozen event catalog** block below). 09 §1.

### Frozen event catalog (5.0.3)

Producers publish `notify.Event{ID, Name, Data, ActorID}` → `EventBus.Publish(orgID, ev)` → XADD `events:{orgId}` MAXLEN ~1000. SSE frame:

```
id: <redis-stream-entry-id>          ← doubles as Last-Event-ID
event: <name>
data: <compact-json of Data, includes "actor_id" and "v":1>
```

| Event name | Data payload keys | Targeting |
|---|---|---|
| `task.created` | task_id, project_key, column_id, title, actor_id, v | broadcast (org) |
| `task.updated` | task_id, project_key, fields[], actor_id, v | broadcast |
| `task.moved` | task_id, project_key, from_column, to_column, rank, actor_id, v | broadcast |
| `task.deleted` | task_id, project_key, actor_id, v | broadcast |
| `task.restored` | task_id, project_key, actor_id, v | broadcast |
| `comment.created` | task_id, comment_id, project_key, actor_id, v | broadcast |
| `member.joined` | user_id, role, actor_id, v | broadcast |
| `member.left` | user_id, actor_id, v | broadcast |
| `member.role_changed` | user_id, role, actor_id, v | broadcast |
| `membership.revoked` | user_id, actor_id, v | targeted (that user hard-redirects out of org) |
| `notification.created` | notification_id, category, user_id, actor_id, v | targeted by user_id (client-side filter) |
| `billing.status_changed` | status, plan_code, actor_id, v | broadcast |
| `resync` | (empty) | server-emitted on replay gap; client invalidates caches |

Wire invariants: `v:1` schema version on every payload; `actor_id` always present so a client skips its own optimistic-applied events (`actor_id === me`); targeted events are filtered client-side (payload carries `user_id`), not by separate streams — one stream per org keeps the consumer model simple. Event-name constants live in `internal/domain/notify/event.go`.

## Section 1 — Migrations (0013 DDL, 0014 RLS)

- [x] 5.1.1 `0013_notifications.up.sql`: `notifications` [T] (id, org_id, user_id, category, title, body, entity_type, entity_id, read_at, created_at) + index (org_id,user_id,read_at). FR-NTF-002. 90-day retention. — two indexes: `notifications_user_idx (org_id,user_id,created_at DESC)` for list, partial `notifications_unread_idx … WHERE read_at IS NULL` for unread-count.
- [x] 5.1.2 `0013`: `notification_prefs` [T] (org_id, user_id, category, email bool, in_app bool, `PK(org_id,user_id,category)`). FR-NTF-004.
- [x] 5.1.3 `0013`: `project_stats_daily` [T] (org_id, project_id, day, completed_count, created_count, per-column counts jsonb, cycle_time_secs, `UNIQUE(project_id,day)`) — backs `stats:rollup` here + analytics reads in Phase 6. FR-AN-001. — column names follow 07 §4 (`column_snapshot`, `avg_cycle_seconds`); `PRIMARY KEY (project_id, day)` + `project_stats_daily_org_idx (org_id, day DESC)`.
- [x] 5.1.4 `0013.down.sql` + `0014_realtime_rls.up/.down.sql`: RLS `tenant_isolation` on all three (copy 0008 pattern).
- [x] 5.1.5 [VERIFY] `migrate up`/`down 2`/`up` clean on 0013/0014. — verified against live compose Postgres: `13/u`,`14/u` → `14/d`,`13/d` → `13/u`,`14/u` all clean.

> **Schema-shape decision (07 §4 vs this tracker conflict).** docs/07 §4 models `notifications` as `(kind, payload jsonb)` and `notification_prefs` as channel-rows `(user_id, category, channel, enabled)` with **no org_id**. This tracker (5.1.1/5.1.2) specifies structured notification columns and a `email/in_app` matrix keyed by `(org_id,user_id,category)`. **Followed the tracker** (same precedent as Phase 4, where 0011 evolved past 07): structured columns give the list endpoint render-ready rows; the prefs matrix keyed by org_id satisfies invariant #1 (every [T] table carries org_id for RLS) — the org_id-less 07 prefs shape could not be RLS-isolated. Recorded here rather than as a silent improvisation.

## Section 2 — Domain (`internal/domain/notify/`)

- [x] 5.2.1 `notify.go`: `Notification` model; `Category` enum (task_assigned, mention, comment, invite_accepted, billing); `Channel` enum (email, in_app); `Pref` model. — package is `internal/domain/notify` (matches `notifyuc`; empty `domain/notification` scaffold removed). Adds `WantsChannel`/`DefaultPref` resolution + `CenterCategories()`; `ProjectStat` rollup grain lives here too.
- [x] 5.2.2 `event.go`: `Event` type (id, name, data, actorID) + event-name constants from the catalog (5.0.3). — `Event.ID` assigned by the bus on publish; `NewEvent` helper; all 13 catalog name constants.
- [x] 5.2.3 `ports.go`: `NotificationRepository`, `PrefRepository` (orgID-first) + `StatsRepository` (project_stats_daily upsert). — NotificationRepository has CreateBatch/List(ListFilter cursor)/UnreadCount/MarkRead/MarkAllRead; PrefRepository GetForUser/Get(send-time)/Upsert; StatsRepository ProjectIDs/ComputeDay/Upsert.
- [x] 5.2.4 `ports.go`: `EventBus` (Publish(orgID, Event), Subscribe(orgID)→channel, Replay(orgID, lastID)→(events, gap bool)). — Subscribe returns (chan, cancel func); Publish returns assigned entry id; Replay returns (events, gap bool).
- [x] 5.2.5 [VERIFY] Unit: pref resolution — opt-out honored; transactional auth-email categories (verify/reset) bypass prefs via whitelist. FR-NTF-003. — `notify_test.go` 6 tests green (default opt-in, opt-out honored, transactional bypass, category validity/transactional, default pref).

## Section 3 — Usecase (`internal/usecase/notifyuc/`)

- [ ] 5.3.1 `service.go`: create notifications, `List/UnreadCount/MarkRead/MarkAllRead`. FR-NTF-002.
- [ ] 5.3.2 Fan-out rules → notif rows + `outbox(email:send)` per opted-in target: task assigned, @mention, comment on task you created/assigned, invitation accepted, billing (OWNER/ADMIN). FR-NTF-002, 09 §3.
- [ ] 5.3.3 `@mention` parse of project members inside `taskuc.AddComment` (fills the existing `TODO(phase5)`); resolve handles → user ids → fan-out. FR-TASK-005 → FR-NTF-002.
- [ ] 5.3.4 After-commit publisher wired into `taskuc`/`projectuc`/`tenantuc`/`billinguc` producers → `EventBus.Publish` (task.*, comment.created, member.*, billing.status_changed). 09 §1.
- [ ] 5.3.5 SSE session usecase: subscribe org stream; on connect with `Last-Event-ID` → `Replay`; gap beyond retention → emit `resync`. FR-NTF-001, 09 §1.
- [ ] 5.3.6 Send-time pref recheck (worker re-reads `notification_prefs`). 09 §3.

## Section 4 — Infra

- [ ] 5.4.1 `internal/infrastructure/redis/eventbus.go`: `Publish` = XADD `events:{org}` MAXLEN ~1000; entry id = event id.
- [ ] 5.4.2 `eventbus.go`: per-org consumer goroutine (XREAD BLOCK) demuxing to in-process subscriber channels (one consumer per org with ≥1 subscriber). 09 §1.
- [ ] 5.4.3 `eventbus.go`: `Replay` via XRANGE from `Last-Event-ID`; id older than retention → gap=true.
- [ ] 5.4.4 `postgres/notification_repo.go` over TenantPool.
- [ ] 5.4.5 `postgres/notification_prefs_repo.go` (get/upsert matrix).
- [ ] 5.4.6 `postgres/stats_repo.go` (project_stats_daily upsert + range read). Add `queries/{notifications,notification_prefs,stats}.sql`; `sqlc generate`.

## Section 5 — HTTP

- [ ] 5.5.1 `handlers/events.go`: `GET /orgs/{orgId}/events` — `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`; `:ka` heartbeat every 25s; `http.Flusher` per event. FR-NTF-001.
- [ ] 5.5.2 `events.go`: read `Last-Event-ID` header → replay then live; emit `event: resync` on gap; respect ctx cancel.
- [ ] 5.5.3 `handlers/notifications.go`: `GET /orgs/{orgId}/notifications` (list, unread/all tabs, paginated).
- [ ] 5.5.4 `notifications.go`: `GET …/notifications/unread-count`, `POST …/notifications/{id}/read`, `POST …/notifications/read-all`.
- [ ] 5.5.5 `router.go`: mount events + notifications under `read(ObjOrg)` O(GUEST); events endpoint keeps the standard auth+tenant chain. FR-NTF-001/002.

## Section 6 — Wiring

- [ ] 5.6.1 `cmd/api/main.go`: build event bus + `notifyuc`; inject publisher into task/project/tenant/billing services; add handlers to `httpx.Deps`.
- [ ] 5.6.2 `cmd/worker/main.go`: wire notification/pref/stats repos + mailer for the new jobs.
- [ ] 5.6.3 SSE consumer goroutine lifecycle managed by the api process (start on first subscriber, stop on last; graceful shutdown drains).

## Section 7 — Jobs (`internal/interface/jobs/`)

- [ ] 5.7.1 `email:send` handler — outbox-driven; re-read prefs; SMTP send retried ×5 exp backoff; Asynq TaskID = outbox id (at-most-once enqueue). 09 §2/§3.
- [ ] 5.7.2 Asynq queues/priorities: `critical`(6) billing sync/outbox drain · `default`(3) emails/notifications · `low`(1) rollups/GC/purges. Configure server. 09 §2.
- [ ] 5.7.3 `stats:rollup` nightly 03:00 → recompute yesterday's `project_stats_daily` (UPSERT, re-runnable). FR-AN-001.
- [ ] 5.7.4 `org:hard_delete` scheduled at org soft-delete +14d → guarded by `purge_after <= now()` recheck (cancels if restored). FR-TEN-008.
- [ ] 5.7.5 `audit:retention` nightly → per-org delete older than plan retention via privileged role (app role lacks DELETE on audit_log). FR-AUD-002.
- [ ] 5.7.6 `webhook:retry` handler (re-invokes billing consumer → dedup via processed_stripe_events; admin UI trigger is Phase 6). FR-ADM-004.
- [ ] 5.7.7 Register all handlers on worker mux + schedule entries.

## Section 8 — Tests

- [ ] 5.8.1 Unit: pref resolution + auth-email whitelist (extends 5.2.5).
- [ ] 5.8.2 Replay/resync: reconnect with stale `Last-Event-ID` beyond retention → `resync`. 09 §4.
- [ ] 5.8.3 Outbox drain: drainer down then up → backlog drains, no duplicate emails (TaskID dedup). 09 §4.
- [ ] 5.8.4 Fan-out: comment with @mention → notification row + `email:send` outbox for opted-in target only.

## Section 9 — E2E verify (dockerized)

- [ ] 5.9.1 [VERIFY] Open SSE (PowerShell/curl-in-container); create/move a task from a second client → `task.created`/`task.moved` received.
- [ ] 5.9.2 [VERIFY] Reconnect with `Last-Event-ID` → missed events replayed.
- [ ] 5.9.3 [VERIFY] @mention in a comment → notification via API + email visible in Mailpit.
- [ ] 5.9.4 [VERIFY] `redis-cli FLUSHALL` → SSE clients get `resync` on reconnect; board self-heals. Write `scratchpad/smoke5.ps1`.

## Section 10 — Commit gate

- [ ] 5.10.1 [VERIFY] `cd backend && go build ./... && go vet ./... && go test ./...` green.
- [ ] 5.10.2 `git commit` (`feat(realtime): phase 5 — SSE stream, notifications, async jobs`); update README Current Position.

---

## Definition of Done (Phase 5)

- [ ] NTF-001..003 (M) ticked; NTF-004 (S) optional (prefs API done, UI is Phase 7).
- [ ] SSE fan-out via Redis Stream, replay + resync working (5.9.1–5.9.2/5.9.4).
- [ ] Notification fan-out + email via outbox; prefs honored at send-time.
- [ ] All 09 §2 jobs registered, idempotent, per-org isolated.
- [ ] E2E green; committed. README advanced to Phase 6.
