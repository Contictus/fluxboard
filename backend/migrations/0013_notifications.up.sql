-- 0013_notifications — Phase 5 (docs/09-REALTIME-JOBS.md, docs/07 §4, FR-NTF-002/004,
-- FR-AN-001). Three tenant-owned tables: the notification center, per-user delivery
-- preferences, and the daily project stats rollup that backs analytics (Phase 6).
-- DDL only; RLS lands in 0014 (the DDL/RLS split, as 0011/0012).
--
-- Schema-shape note (reconciles a conflict between docs/07 §4 and this phase's build
-- tracker — recorded in docs/build/PHASE-5 §1): notifications carry structured
-- (category,title,body,entity_type,entity_id) columns rather than a single jsonb
-- payload, so the list endpoint (FR-NTF-002) returns render-ready rows and the SSE
-- notification.created event carries just {id,category}. notification_prefs is the
-- matrix shape (email/in_app booleans keyed by category) WITH org_id — every
-- tenant-owned table carries org_id for RLS (non-negotiable invariant #1), so the
-- org_id-less 07 §4 prefs shape is not used. Times = timestamptz UTC.

-- notifications [T] — in-app notification center (FR-NTF-002). 90-day retention
-- enforced by the audit:retention-style nightly job is out of scope here; rows are
-- pruned by a future cleanup. entity_type/entity_id deep-link the client to the
-- source (task/comment/etc). read_at NULL ⇒ unread.
CREATE TABLE notifications (                           -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category    text NOT NULL,                           -- task_assigned | mention | comment | invite_accepted | billing
  title       text NOT NULL,
  body        text NOT NULL DEFAULT '',
  entity_type text,                                    -- task | comment | project | org | subscription
  entity_id   text,
  read_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
-- List (org,user, newest first) and unread-count both hit this; read_at leads so the
-- partial-friendly unread scan stays tight.
CREATE INDEX notifications_user_idx ON notifications (org_id, user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (org_id, user_id) WHERE read_at IS NULL;

-- notification_prefs [T] — per-user delivery matrix (FR-NTF-004). One row per
-- (org,user,category); email/in_app toggles. Absent row ⇒ defaults (both true) applied
-- in the domain. Transactional auth emails (verify/reset) bypass this table via a
-- category whitelist in the domain, never stored here.
CREATE TABLE notification_prefs (                      -- [T]
  org_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category text NOT NULL,                              -- matches notifications.category
  email    boolean NOT NULL DEFAULT true,
  in_app   boolean NOT NULL DEFAULT true,
  PRIMARY KEY (org_id, user_id, category)
);

-- project_stats_daily [T] — nightly rollup grain (FR-AN-001). One row per
-- (project,day); set-semantics UPSERT ⇒ stats:rollup is re-runnable. Backs the
-- analytics reads in Phase 6. Column names follow docs/07 §4 (column_snapshot,
-- avg_cycle_seconds). org_id carried for RLS.
CREATE TABLE project_stats_daily (                     -- [T]
  org_id            uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  project_id        uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  day               date NOT NULL,
  created_count     int NOT NULL DEFAULT 0,
  completed_count   int NOT NULL DEFAULT 0,
  column_snapshot   jsonb NOT NULL DEFAULT '{}',       -- {"column_uuid": task_count}
  avg_cycle_seconds bigint,
  PRIMARY KEY (project_id, day)
);
CREATE INDEX project_stats_daily_org_idx ON project_stats_daily (org_id, day DESC);
