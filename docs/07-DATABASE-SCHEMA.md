# 07 — Database Schema (PostgreSQL 16)

Conventions: UUIDv7 PKs (`pkg/uuidv7`, time-ordered → index-friendly),
`timestamptz` everywhere, `created_at/updated_at` on all tables
(updated_at via trigger), soft delete = `deleted_at timestamptz`.
Migrations: golang-migrate, `NNNN_name.up.sql/.down.sql`, every migration
reversible. RLS policy pattern from 05 §4 applied to every table marked **[T]**
(tenant-owned).

## 1. Global Tables (no RLS)

```sql
CREATE TABLE users (
  id              uuid PRIMARY KEY,
  email           citext NOT NULL UNIQUE,
  password_hash   text,                          -- NULL for OAuth-only accounts
  name            text NOT NULL DEFAULT '',
  avatar_key      text,                          -- MinIO object key
  email_verified  boolean NOT NULL DEFAULT false,
  platform_role   text NOT NULL DEFAULT 'user'
                  CHECK (platform_role IN ('user','admin')),
  totp_secret     text,                          -- encrypted at rest (AES-GCM, key from env)
  totp_enabled    boolean NOT NULL DEFAULT false,
  locked_at       timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_identities (
  id           uuid PRIMARY KEY,
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider     text NOT NULL CHECK (provider IN ('google')),
  provider_sub text NOT NULL,
  UNIQUE (provider, provider_sub),
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (                          -- refresh token families (04 §3)
  id            uuid PRIMARY KEY,                -- = one refresh token instance
  family_id     uuid NOT NULL,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash    bytea NOT NULL UNIQUE,           -- SHA-256
  user_agent    text, ip inet,
  expires_at    timestamptz NOT NULL,
  rotated_at    timestamptz,
  revoked_at    timestamptz,
  revoke_reason text,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON sessions (family_id);
CREATE INDEX ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE one_time_tokens (                   -- verify/reset/invite share shape
  id         uuid PRIMARY KEY,
  purpose    text NOT NULL CHECK (purpose IN ('email_verify','password_reset','email_change')),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  payload    jsonb NOT NULL DEFAULT '{}',        -- e.g. new email
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recovery_codes (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash  bytea NOT NULL,
  used_at    timestamptz,
  PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE plans (
  code                 text PRIMARY KEY,          -- 'free' | 'pro' | 'business'
  name                 text NOT NULL,
  stripe_product_id    text,
  seat_price_id        text,
  metered_price_ids    jsonb NOT NULL DEFAULT '{}',  -- {"storage": "...", "api_calls": "..."}
  max_members          int  NOT NULL,             -- -1 unlimited
  max_projects         int  NOT NULL,
  max_storage_bytes    bigint NOT NULL,
  api_rate_per_min     int  NOT NULL,
  audit_retention_days int  NOT NULL,
  metered              boolean NOT NULL DEFAULT false
);

CREATE TABLE processed_stripe_events (           -- idempotency ledger (06 §4)
  event_id     text PRIMARY KEY,
  type         text NOT NULL,
  payload      jsonb NOT NULL,
  handled      boolean NOT NULL DEFAULT true,
  error        text,
  processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox (                            -- transactional outbox → Asynq (06 §4)
  id          uuid PRIMARY KEY,
  task_type   text NOT NULL,
  payload     jsonb NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  drained_at  timestamptz
);
CREATE INDEX ON outbox (created_at) WHERE drained_at IS NULL;
```

## 2. Tenant Root

```sql
CREATE TABLE organizations (
  id          uuid PRIMARY KEY,
  slug        citext NOT NULL UNIQUE,
  name        text NOT NULL,
  logo_key    text,
  stripe_customer_id text UNIQUE,
  deleted_at  timestamptz,                       -- soft delete + 14d grace
  purge_after timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
-- organizations itself: no RLS (must be readable pre-context to RESOLVE context);
-- access mediated by membership joins in queries.

CREATE TABLE slug_history (
  old_slug   citext PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL                -- 30d 301 window (FR-TEN-007)
);

CREATE TABLE memberships (                       -- [T]
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       text NOT NULL CHECK (role IN ('OWNER','ADMIN','MEMBER','GUEST')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, user_id)
);
CREATE INDEX ON memberships (user_id);           -- "my orgs" lookup

CREATE TABLE invitations (                       -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email       citext NOT NULL,
  role        text NOT NULL CHECK (role IN ('ADMIN','MEMBER','GUEST')),
  token_hash  bytea NOT NULL UNIQUE,
  invited_by  uuid NOT NULL REFERENCES users(id),
  expires_at  timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, email)                          -- one pending per address
);

CREATE TABLE subscriptions (                     -- [T] Stripe mirror (06 §2)
  org_id                 uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
  stripe_subscription_id text UNIQUE,
  plan_code              text NOT NULL REFERENCES plans(code) DEFAULT 'free',
  status                 text NOT NULL DEFAULT 'none'
        CHECK (status IN ('none','trialing','active','past_due','unpaid','canceled')),
  seat_quantity          int NOT NULL DEFAULT 0,
  cancel_at_period_end   boolean NOT NULL DEFAULT false,
  current_period_end     timestamptz,
  last_stripe_event_at   timestamptz,            -- out-of-order guard
  updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invoices (                          -- [T] mirror
  id               uuid PRIMARY KEY,
  org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  stripe_invoice_id text NOT NULL UNIQUE,
  number           text,
  status           text NOT NULL,
  amount_due       bigint NOT NULL,              -- minor units (ADR-012)
  amount_paid      bigint NOT NULL DEFAULT 0,
  currency         text NOT NULL DEFAULT 'usd',
  hosted_url       text, pdf_url text,
  period_start     timestamptz, period_end timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE usage_records (                     -- [T] (06 §5)
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  metric      text NOT NULL CHECK (metric IN ('active_members','storage_bytes','api_calls')),
  period_date date NOT NULL,
  value       bigint NOT NULL,
  pushed_to_stripe_at timestamptz,
  PRIMARY KEY (org_id, metric, period_date)
);
```

## 3. Project Domain — all **[T]**

```sql
CREATE TABLE projects (
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[A-Z]{2,6}$'),
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  color       text NOT NULL DEFAULT '#6366f1',
  visibility  text NOT NULL DEFAULT 'org' CHECK (visibility IN ('org','private')),
  archived_at timestamptz,
  task_seq    int NOT NULL DEFAULT 0,            -- PAY-123 counter
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, key)
);

CREATE TABLE project_members (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id     uuid NOT NULL,                      -- denormalized for RLS
  role       text NOT NULL CHECK (role IN ('LEAD','CONTRIBUTOR','VIEWER')),
  PRIMARY KEY (project_id, user_id)
);

CREATE TABLE columns (
  id         uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  org_id     uuid NOT NULL,
  name       text NOT NULL,
  position   int  NOT NULL,
  wip_limit  int,
  UNIQUE (project_id, position) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE tasks (
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL,
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  column_id   uuid NOT NULL REFERENCES columns(id),
  number      int  NOT NULL,                     -- from projects.task_seq
  title       text NOT NULL CHECK (char_length(title) <= 200),
  description text NOT NULL DEFAULT '',
  priority    text NOT NULL DEFAULT 'none'
              CHECK (priority IN ('urgent','high','medium','low','none')),
  assignee_id uuid REFERENCES users(id) ON DELETE SET NULL,
  due_date    date,
  rank        text NOT NULL,                     -- LexoRank (ADR-009)
  search_tsv  tsvector GENERATED ALWAYS AS
              (to_tsvector('simple', title || ' ' || description)) STORED,
  deleted_at  timestamptz,                       -- trash (FR-TASK-009)
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, number)
);
CREATE INDEX tasks_board_idx  ON tasks (column_id, rank) WHERE deleted_at IS NULL;
CREATE INDEX tasks_assignee_idx ON tasks (org_id, assignee_id) WHERE deleted_at IS NULL;
CREATE INDEX tasks_search_idx ON tasks USING gin (search_tsv);

CREATE TABLE subtasks (
  id       uuid PRIMARY KEY,
  org_id   uuid NOT NULL,
  task_id  uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  title    text NOT NULL,
  done     boolean NOT NULL DEFAULT false,
  position int NOT NULL
);

CREATE TABLE labels (
  id     uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name   text NOT NULL CHECK (char_length(name) <= 30),
  color  text NOT NULL,
  UNIQUE (org_id, name)
);
CREATE TABLE task_labels (
  task_id  uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  label_id uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  org_id   uuid NOT NULL,
  PRIMARY KEY (task_id, label_id)
);

CREATE TABLE task_relations (                    -- FR-TASK-010
  blocker_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  blocked_id uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  org_id     uuid NOT NULL,
  PRIMARY KEY (blocker_id, blocked_id),
  CHECK (blocker_id <> blocked_id)
);

CREATE TABLE comments (
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL,
  task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  author_id  uuid NOT NULL REFERENCES users(id),
  body       text NOT NULL,
  edited_at  timestamptz,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON comments (task_id, created_at);

CREATE TABLE attachments (                       -- FR-TASK-006
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL,
  task_id     uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  object_key  text NOT NULL UNIQUE,              -- {org_id}/{task_id}/{uuid}/{filename}
  filename    text NOT NULL,
  mime        text NOT NULL,
  size_bytes  bigint NOT NULL,
  status      text NOT NULL DEFAULT 'pending'    -- pending → confirmed (orphan GC on pending)
              CHECK (status IN ('pending','confirmed')),
  uploaded_by uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE task_activity (                     -- FR-TASK-002
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL,
  task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  actor_id   uuid NOT NULL REFERENCES users(id),
  field      text NOT NULL,
  old_value  jsonb, new_value jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON task_activity (task_id, created_at DESC);
```

## 4. Notifications, Audit, API Keys, Analytics

```sql
CREATE TABLE notifications (                     -- [T]
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       text NOT NULL,                      -- task_assigned | mention | comment | billing | invite_accepted
  payload    jsonb NOT NULL,                     -- {task_id, project_key, actor_name, ...}
  read_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON notifications (user_id, org_id, created_at DESC) WHERE read_at IS NULL;

CREATE TABLE notification_prefs (
  user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category text NOT NULL,
  channel  text NOT NULL CHECK (channel IN ('email','inapp')),
  enabled  boolean NOT NULL DEFAULT true,
  PRIMARY KEY (user_id, category, channel)
);

CREATE TABLE audit_log (                         -- FR-AUD-001/002
  id                  uuid PRIMARY KEY,
  org_id              uuid,                      -- NULL for platform-level events
  actor_user_id       uuid,
  impersonator_user_id uuid,
  action              text NOT NULL,             -- 'auth.login', 'member.role_changed', ...
  target_type         text, target_id text,
  metadata            jsonb NOT NULL DEFAULT '{}',
  ip inet, user_agent text,
  severity            text NOT NULL DEFAULT 'info'
                      CHECK (severity IN ('info','warning','security')),
  created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON audit_log (org_id, created_at DESC);
CREATE INDEX ON audit_log (severity, created_at DESC) WHERE severity = 'security';
REVOKE UPDATE, DELETE ON audit_log FROM fluxboard_app;   -- append-only for app role

CREATE TABLE api_keys (                          -- [T] FR-API-001
  id         uuid PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name       text NOT NULL,
  key_hash   bytea NOT NULL UNIQUE,
  prefix     text NOT NULL,                      -- first 12 chars for display
  scopes     text[] NOT NULL DEFAULT '{read}',
  created_by uuid NOT NULL REFERENCES users(id),
  last_used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_stats_daily (               -- [T] FR-AN-001 rollup
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  org_id     uuid NOT NULL,
  day        date NOT NULL,
  created_count int NOT NULL DEFAULT 0,
  completed_count int NOT NULL DEFAULT 0,
  column_snapshot jsonb NOT NULL DEFAULT '{}',   -- {"col_uuid": task_count}
  avg_cycle_seconds bigint,
  PRIMARY KEY (project_id, day)
);

CREATE TABLE feature_flags (                     -- FR-ADM-006
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  flag   text NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  PRIMARY KEY (org_id, flag)
);
```

## 5. Migration Order

```
0001 extensions (citext, pgcrypto)     0017 automation_rules
0002 users, oauth, sessions,            0018 RLS automation_rules
     one_time_tokens, recovery_codes   0019 time_entries
0003 organizations, slug_history,       0020 RLS time_entries
     memberships, invitations          0021 sprints (+ tasks.sprint_id)
0004 RLS tenancy [T]                    0022 RLS sprints
0005 audit_log (append-only)            0023 custom_fields + values
0006 session last_used                   0024 RLS custom_fields
0007 projects, boards, columns, tasks,  0025 task_links
     subtasks, labels, comments,       0026 RLS task_links
     activity                          0027 project_forms (+ app_form_by_token)
0008 RLS phase-3a [T]                   0028 RLS project_forms
0009 trash, FTS tsvector, attachments   0029 tasks.start_date
0010 RLS attachments                    0030 ai_runs, ai_risks
0011 plans, subscriptions, invoices,    0031 RLS ai_runs, ai_risks
     processed_stripe_events,          0032 ai_runs key_id (API-key callers)
     usage_records, outbox
0012 RLS billing [T]
0013 notifications, prefs, stats_daily
0014 RLS realtime [T]
0015 api_keys, flags, overrides
0016 RLS admin [T]
```

Convention: DDL lands in the odd migration, RLS in the following even one,
so the isolation layer reviews as one artifact. Next free number: **0033**.
RLS policies live in dedicated migrations so the isolation layer is
reviewable as one artifact.
