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

- [ ] 5.0.1 [VERIFY] Confirm Phase-4 `outbox` table + `outbox:drain` job exist (dependency). If billing was stubbed, ensure outbox landed regardless.
- [ ] 5.0.2 [DECISION] Confirm fan-out = **Redis Stream per org** (`events:{orgId}`, XADD MAXLEN ~1000), per 09 §1 — records the horizontal-scale rationale.
- [ ] 5.0.3 Freeze the event catalog + wire format in this file: `task.created|updated|moved|deleted|restored`, `comment.created`, `member.joined|left|role_changed`, `membership.revoked`, `notification.created`, `billing.status_changed`, `resync`. 09 §1.

## Section 1 — Migrations (0013 DDL, 0014 RLS)

- [ ] 5.1.1 `0013_notifications.up.sql`: `notifications` [T] (id, org_id, user_id, category, title, body, entity_type, entity_id, read_at, created_at) + index (org_id,user_id,read_at). FR-NTF-002. 90-day retention.
- [ ] 5.1.2 `0013`: `notification_prefs` [T] (org_id, user_id, category, email bool, in_app bool, `PK(org_id,user_id,category)`). FR-NTF-004.
- [ ] 5.1.3 `0013`: `project_stats_daily` [T] (org_id, project_id, day, completed_count, created_count, per-column counts jsonb, cycle_time_secs, `UNIQUE(project_id,day)`) — backs `stats:rollup` here + analytics reads in Phase 6. FR-AN-001.
- [ ] 5.1.4 `0013.down.sql` + `0014_realtime_rls.up/.down.sql`: RLS `tenant_isolation` on all three (copy 0008 pattern).
- [ ] 5.1.5 [VERIFY] `migrate up`/`down 2`/`up` clean on 0013/0014.

## Section 2 — Domain (`internal/domain/notify/`)

- [ ] 5.2.1 `notify.go`: `Notification` model; `Category` enum (task_assigned, mention, comment, invite_accepted, billing); `Channel` enum (email, in_app); `Pref` model.
- [ ] 5.2.2 `event.go`: `Event` type (id, name, data, actorID) + event-name constants from the catalog (5.0.3).
- [ ] 5.2.3 `ports.go`: `NotificationRepository`, `PrefRepository` (orgID-first) + `StatsRepository` (project_stats_daily upsert).
- [ ] 5.2.4 `ports.go`: `EventBus` (Publish(orgID, Event), Subscribe(orgID)→channel, Replay(orgID, lastID)→(events, gap bool)).
- [ ] 5.2.5 [VERIFY] Unit: pref resolution — opt-out honored; transactional auth-email categories (verify/reset) bypass prefs via whitelist. FR-NTF-003.

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
