-- 0015_admin — Phase 6 (docs/build/PHASE-6-ADMIN-OBS.md §1, FR-ADM-002/006, FR-API-001).
-- Three tenant-owned tables (org-scoped API keys, feature flags, entitlement
-- overrides) plus one column added to the global plans reference table. DDL only;
-- RLS for the [T] tables lands in 0016 (the DDL/RLS split, as 0011/0012, 0013/0014).
--
-- Build-tracker conflicts recorded (PHASE-6 §0): 6.1.1 (users.platform_role) and the
-- 6.0.2/6.1.5 audit_log columns are NOT added here — platform_role/totp_secret/
-- totp_enabled already exist since 0002, and audit_log (0005) already carries
-- impersonator_user_id, severity and a nullable org_id. So this migration lands only
-- the genuinely new surface. Money = integer minor units (invariant #5); times UTC.

-- plans.monthly_price — the recurring price of the tier in minor units (cents), the
-- local source for the admin MRR aggregate (FR-ADM-002). plans stores only Stripe
-- price *IDs* otherwise, which are unusable for a local MRR sum in MODE=stub; this
-- column is seeded by cmd/stripeseed. free=0.
ALTER TABLE plans ADD COLUMN monthly_price bigint NOT NULL DEFAULT 0;

-- api_keys [T] — org-scoped API keys, a second auth path (FR-API-001). The secret is
-- shown once at creation; only its SHA-256 hash is stored. prefix is the public,
-- searchable head ('fbk_live_' + a short id) shown in the UI to identify a key without
-- revealing it. scopes gates read vs write. revoked_at NULL ⇒ active.
CREATE TABLE api_keys (                                -- [T]
  id          uuid PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  prefix      text NOT NULL,                           -- 'fbk_live_ab12cd34' (public identifier)
  key_hash    text NOT NULL UNIQUE,                    -- SHA-256 of the full secret
  name        text NOT NULL,
  scopes      text[] NOT NULL DEFAULT '{}',            -- {'read'} | {'read','write'}
  created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  last_used_at timestamptz,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_keys_org_idx ON api_keys (org_id, created_at DESC);

-- feature_flags [T] — per-org feature toggles (FR-ADM-006). One row per (org,flag);
-- absent row ⇒ flag off (default applied in the domain).
CREATE TABLE feature_flags (                           -- [T]
  org_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  flag    text NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  PRIMARY KEY (org_id, flag)
);

-- entitlement_overrides [T] — platform-admin overrides of a plan entitlement for a
-- single org (FR-ADM-002), e.g. a bespoke seat cap. key matches an entitlement field;
-- value is stored as text and parsed by the resolver. note + created_by capture who/why.
CREATE TABLE entitlement_overrides (                   -- [T]
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  key        text NOT NULL,                            -- 'max_members' | 'max_projects' | 'api_rate_per_min' | ...
  value      text NOT NULL,
  note       text NOT NULL DEFAULT '',
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, key)
);
