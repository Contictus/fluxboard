-- 0002_auth — global (non-tenant) auth tables from docs/07-DATABASE-SCHEMA.md §1.
-- These live outside RLS: a user exists before/independent of any org.

-- updated_at trigger, reused by every table with an updated_at column.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

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
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

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
  user_agent    text NOT NULL DEFAULT '',        -- NOT NULL DEFAULT '' for clean codegen
  ip            inet,                            -- read via host(ip); NULL when unknown
  expires_at    timestamptz NOT NULL,
  rotated_at    timestamptz,
  revoked_at    timestamptz,
  revoke_reason text,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_family_idx ON sessions (family_id);
CREATE INDEX sessions_user_active_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE one_time_tokens (                   -- verify/reset share this shape
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
