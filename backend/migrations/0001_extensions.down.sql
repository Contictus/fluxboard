-- Reverse 0001_extensions. Safe because no objects depend on these yet in
-- Phase 0. Later migrations that add citext/pgcrypto-dependent objects own
-- their own reversibility.
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS citext;
