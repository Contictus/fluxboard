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

- [x] 5.3.1 `service.go`: create notifications, `List/UnreadCount/MarkRead/MarkAllRead`. FR-NTF-002. — plus `GetPrefs`/`SetPref` (matrix, defaults filled) for FR-NTF-004; list capped at 50, cursor by created_at.
- [x] 5.3.2 Fan-out rules → notif rows + `outbox(email:send)` per opted-in target: task assigned, @mention, comment on task you created/assigned, invitation accepted, billing (OWNER/ADMIN). FR-NTF-002, 09 §3. — `deliver()` reads each target's pref: in-app row when `WantsChannel(…in_app)`, email outbox when `WantsChannel(…email)` (rechecked at send). `FanOutComment` (mention∪comment-targets), `NotifyAssigned`, `NotifyTargets` (invite/billing). Billing owner-targeting handled by the existing billing outbox path; notify.NotifyTargets covers invite-accepted.
- [x] 5.3.3 `@mention` parse of project members inside `taskuc.AddComment` (fills the existing `TODO(phase5)`); resolve handles → user ids → fan-out. FR-TASK-005 → FR-NTF-002. — handle = email local-part (users have no username — noted in §3 decision); `mentionRE` + `Directory.ProjectMembers`; comment targets = task creator + assignee passed by taskuc (keeps notifyuc task-repo-free).
- [x] 5.3.4 After-commit publisher wired into `taskuc`/`projectuc`/`tenantuc`/`billinguc` producers → `EventBus.Publish` (task.*, comment.created, member.*, billing.status_changed). 09 §1. — taskuc: task.created/updated/deleted/restored + comment.created; projectuc: task.moved; tenantuc: member.joined/left/role_changed + membership.revoked; billinguc: billing.status_changed (on SubscriptionCh). Payloads carry `project_id` (not project_key — avoids an extra lookup; frontend is Phase 7). Producers depend on the `notify.EventBus` domain port directly (no usecase→usecase import); fan-out via locally-declared `Notifier` interfaces satisfied by notifyuc.Service.
- [x] 5.3.5 SSE session usecase: subscribe org stream; on connect with `Last-Event-ID` → `Replay`; gap beyond retention → emit `resync`. FR-NTF-001, 09 §1. — `notifyuc.StreamInit` (empty id ⇒ no backlog; gap ⇒ resync=true) + `Subscribe` (channel + cancel); nil bus tolerated.
- [x] 5.3.6 Send-time pref recheck (worker re-reads `notification_prefs`). 09 §3. — email outbox row created at fan-out; the email:send job rechecks `PrefRepository.Get(user,category)` before sending (implemented in §7).

## Section 4 — Infra

- [x] 5.4.1 `internal/infrastructure/redis/eventbus.go`: `Publish` = XADD `events:{org}` MAXLEN ~1000; entry id = event id. — package `redisx`; injects `actor_id` + `v:1` into every payload (wire invariants); `Approx` MAXLEN ~1000.
- [x] 5.4.2 `eventbus.go`: per-org consumer goroutine (XREAD BLOCK) demuxing to in-process subscriber channels (one consumer per org with ≥1 subscriber). 09 §1. — `orgFanout` starts consumer on first `Subscribe`, stops on last `cancel`; `readBlock`=25s; non-blocking dispatch drops for a slow client (recovers via reconnect/replay).
- [x] 5.4.3 `eventbus.go`: `Replay` via XRANGE from `Last-Event-ID`; id older than retention → gap=true. — `XRangeN(-,+,1)` oldest check; `idLess(lastID,oldest)` ⇒ gap; else exclusive `("+lastID` XRange.
- [x] 5.4.4 `postgres/notification_repo.go` over TenantPool. — CreateBatch/List(cursor+OnlyUnread)/UnreadCount/MarkRead(0 rows⇒`domain.ErrNotFound`)/MarkAllRead; nullable entity_type/id via `strPtr`.
- [x] 5.4.5 `postgres/notification_prefs_repo.go` (get/upsert matrix). — GetForUser(map)/Get(`pgx.ErrNoRows`⇒ok=false)/Upsert. Plus `directory_repo.go` (`DirUser` DTO → adapted to `notifyuc.Directory` in cmd wiring, keeps postgres free of a usecase import) and `OutboxRepo.InsertEmail` (reuses `insertOutbox`, kind `email:send`).
- [x] 5.4.6 `postgres/stats_repo.go` (project_stats_daily upsert + range read). Add `queries/{notifications,notification_prefs,stats}.sql`; `sqlc generate`. — ComputeDay: created count over `[day,day+1)` UTC + per-column open snapshot (`column_id`→count); CompletedCount/AvgCycleSeconds deferred to Phase 6 (activity mining); Upsert marshals snapshot jsonb.

## Section 5 — HTTP

- [x] 5.5.1 `handlers/events.go`: `GET /orgs/{orgId}/events` — `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`; `:ka` heartbeat every 25s; `http.Flusher` per event. FR-NTF-001. — `EventHandlers.Stream`; `http.Flusher` asserted (non-streaming writer ⇒ 422); frame writer `id:`/`event:`/`data:` (compact-JSON Data).
- [x] 5.5.2 `events.go`: read `Last-Event-ID` header → replay then live; emit `event: resync` on gap; respect ctx cancel. — `Last-Event-ID` header (or `?last`) → `StreamInit`; resync frame `event: resync\ndata: {}`; select loop on ctx.Done / event chan / 25s ticker; `defer cancel()` releases the bus subscription.
- [x] 5.5.3 `handlers/notifications.go`: `GET /orgs/{orgId}/notifications` (list, unread/all tabs, paginated). — `?unread=1`, `?limit=N` (≤50), `?before=RFC3339` cursor; userID from TenantContext (never a param).
- [x] 5.5.4 `notifications.go`: `GET …/notifications/unread-count`, `POST …/notifications/{id}/read`, `POST …/notifications/read-all`. — plus `GET/PUT …/notifications/prefs` (matrix, FR-NTF-004); MarkRead 404s on a foreign/absent id.
- [x] 5.5.5 `router.go`: mount events + notifications under `read(ObjOrg)` O(GUEST); events endpoint keeps the standard auth+tenant chain. FR-NTF-001/002. — mounted in the `/orgs/{orgId}` group under `read(ObjOrg)`; `Deps.Events`/`Deps.Notifications` nil ⇒ routes skipped (tests).

## Section 6 — Wiring

- [x] 5.6.1 `cmd/api/main.go`: build event bus + `notifyuc`; inject publisher into task/project/tenant/billing services; add handlers to `httpx.Deps`. — `redisx.NewEventBus` + `notifyuc.New`; `Events`/`Notifier` injected into task/tenant, `Events` into project, `Bus` into billing; `notifyDirectory` adapter (postgres DTO → notifyuc.UserRef) keeps postgres usecase-import-free.
- [x] 5.6.2 `cmd/worker/main.go`: wire notification/pref/stats repos + mailer for the new jobs. — `jobs.NewNotify` wired with `PrefRepo`/`StatsRepo` + shared mailer; `BillingEmail: billingJobs.HandleEmailSend` delegates billing payloads; registered on the mux; TODO removed.
- [x] 5.6.3 SSE consumer goroutine lifecycle managed by the api process (start on first subscriber, stop on last; graceful shutdown drains). — handled inside `EventBus.Subscribe/consume` (per-org goroutine starts on first sub, stops on last cancel); the SSE handler `defer cancel()`s on disconnect and returns on `ctx.Done()`, so `srv.Shutdown` cancellation drains live streams.

## Section 7 — Jobs (`internal/interface/jobs/`)

- [x] 5.7.1 `email:send` handler — outbox-driven; re-read prefs; SMTP send retried ×5 exp backoff; Asynq TaskID = outbox id (at-most-once enqueue). 09 §2/§3. — `jobs/notifications.go Notify.HandleEmailSend`: one handler owns the shared `email:send` kind, dispatching on payload (`user_id` ⇒ notification, else billing → `Billing.HandleEmailSend`). Notification path rechecks `PrefRepository.Get` at send-time (transactional bypass via `WantsChannel`), then `mailer.SendNotification`. TaskID=`outbox:{id}` + `MaxRetry(5)` set by the drainer (`enqueueOutboxItem`). `EmailPayload` gained `org_id` so the worker can scope the pref recheck.
- [x] 5.7.2 Asynq queues/priorities: `critical`(6) billing sync/outbox drain · `default`(3) emails/notifications · `low`(1) rollups/GC/purges. Configure server. 09 §2. — `jobs.Queues()` (critical:6/default:3/low:1) already wired on the asynq server (Phase 4); email:send enqueues on `default`, stats:rollup on `low`.
- [x] 5.7.3 `stats:rollup` nightly 03:00 → recompute yesterday's `project_stats_daily` (UPSERT, re-runnable). FR-AN-001. — `Notify.HandleStatsRollup`: `forEachOrg` → `StatsRepo.ProjectIDs` → `ComputeDay(yesterday UTC)` → `Upsert`; scheduled `15 3 * * *` (offset from trash-purge 03:00 / GC 03:30) on queue `low`.
- [ ] 5.7.4 `org:hard_delete` scheduled at org soft-delete +14d → guarded by `purge_after <= now()` recheck (cancels if restored). FR-TEN-008. — **DEFERRED to Phase 6** (see §7 deferral note). No infra yet: needs a privileged cross-tenant purge query (`ListOrgsToPurge` + cascade `HardDeleteOrg`) — `ListActiveOrgIDs` only yields non-deleted orgs. Cascade delete across every [T] table under RLS is a privileged op; improvising DB grants here would violate CLAUDE.md §"don't silently improvise".
- [ ] 5.7.5 `audit:retention` nightly → per-org delete older than plan retention via privileged role (app role lacks DELETE on audit_log). FR-AUD-002. — **DEFERRED to Phase 6** (see note). The spec itself says the app role lacks DELETE on `audit_log`; the privileged retention role is not provisioned yet (Phase 6 admin/observability).
- [ ] 5.7.6 `webhook:retry` handler (re-invokes billing consumer → dedup via processed_stripe_events; admin UI trigger is Phase 6). FR-ADM-004. — **DEFERRED to Phase 6** (see note). No raw-webhook store exists to replay from, and the doc scopes the admin trigger to Phase 6.
- [x] 5.7.7 Register all handlers on worker mux + schedule entries. — `notifyJobs.Register(mux)` (email:send + stats:rollup); billing.Register no longer wires email:send (Notify owns it, delegating billing payloads back). `stats:rollup` added to `jobs.Schedule()`.

> **§7 deferral note (5.7.4–5.7.6).** Three jobs are deferred to Phase 6 because the infrastructure they need does not exist in the Phase-5 tree and building it here would mean improvising privileged/cross-tenant DB operations (against CLAUDE.md §"When Reality Conflicts With These Docs"): (a) `org:hard_delete` needs a cross-tenant purge query + cascade delete under a privileged role; (b) `audit:retention` needs a privileged DELETE role the app role explicitly lacks per the spec; (c) `webhook:retry` needs a raw-webhook store to replay and an admin trigger the spec itself scopes to Phase 6. The email/realtime/rollup backbone (5.7.1–5.7.3/5.7.7) — the NTF-001..003 (M) requirements — is complete and green. Phase 6 (admin + observability) owns the privileged roles and admin triggers these three need.

## Section 8 — Tests

- [x] 5.8.1 Unit: pref resolution + auth-email whitelist (extends 5.2.5). — `domain/notify/notify_test.go` (default opt-in, opt-out honored, transactional bypass) + the send-time recheck exercised in `jobs/notifications_test.go` (opted-out ⇒ not mailed).
- [x] 5.8.2 Replay/resync: reconnect with stale `Last-Event-ID` beyond retention → `resync`. 09 §4. — `notifyuc/service_test.go`: `StreamInit` empty-id ⇒ no backlog, valid id ⇒ replays, gap ⇒ resync=true+no backlog.
- [x] 5.8.3 Outbox drain: drainer down then up → backlog drains, no duplicate emails (TaskID dedup). 09 §4. — covered by `jobs/billing_test.go` (`TestOutboxDrainEnqueuesWithTaskID`, `TestOutboxDrainTaskIDConflictIsSuccess`): TaskID=`outbox:{id}`, conflict treated as success.
- [x] 5.8.4 Fan-out: comment with @mention → notification row + `email:send` outbox for opted-in target only. — `notifyuc/service_test.go TestFanOutComment_MentionRowAndEmailForOptedInOnly`: @mention + comment-target rows, actor never self-notified, email outbox only for the email-opted-in target.

## Section 9 — E2E verify (dockerized)

- [x] 5.9.1 [VERIFY] Open SSE (PowerShell/curl-in-container); create/move a task from a second client → `task.created`/`task.moved` received. — `scratchpad/smoke5.ps1` green against `make up`; both frames received on the owner's stream.
- [x] 5.9.2 [VERIFY] Reconnect with `Last-Event-ID` → missed events replayed. — reconnect from the first event id replays `task.moved` (XRANGE backlog).
- [x] 5.9.3 [VERIFY] @mention in a comment → notification via API + email visible in Mailpit. — `@member` comment → member's `GET /notifications?unread=1` non-empty **and** a "You were mentioned" email in Mailpit (fan-out → outbox → drain → email:send, pref-rechecked).
- [x] 5.9.4 [VERIFY] `redis-cli FLUSHALL` → SSE clients get `resync` on reconnect; board self-heals. Write `scratchpad/smoke5.ps1`. — `docker exec … redis-cli FLUSHALL` then reconnect with a stale id → `event: resync`. **Two source bugs found + fixed by this run:** (a) the logging `statusWriter` masked `http.Flusher` (SSE returned 422) — added a `Flush()` passthrough; (b) `EventBus.Replay` treated an empty (flushed) stream as "no gap" — since Replay only runs with a real Last-Event-ID, an empty stream now returns gap=true so the client resyncs.

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
