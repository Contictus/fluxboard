-- 0006_session_last_used — track when a session last authenticated a request
-- (FR-AUTH-008 "last-used" column in GET /auth/sessions). Set to now() on create
-- and advanced by the auth middleware at most once per cache-TTL (≤60s) on the
-- session-liveness backfill path, so it costs no per-request write.

ALTER TABLE sessions ADD COLUMN last_used_at timestamptz NOT NULL DEFAULT now();
