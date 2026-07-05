-- 0003_tenancy — tenant root tables from docs/07-DATABASE-SCHEMA.md §2:
-- organizations (the tenant), slug_history (301 window), memberships and
-- invitations. RLS for the tenant-owned tables ([T]) lands in 0004 so the
-- isolation layer is reviewable as one artifact (docs/07 §5, 05 §4).
--
-- Billing tables (plans, subscriptions, invoices, usage_records) are Phase 4;
-- the invitation seat-entitlement 402 gate (docs/05 §5) is deferred with them.

CREATE TABLE organizations (
  id                 uuid PRIMARY KEY,
  slug               citext NOT NULL UNIQUE,
  name               text NOT NULL,
  logo_key           text,
  stripe_customer_id text UNIQUE,
  deleted_at         timestamptz,                 -- soft delete + grace (invariant #7)
  purge_after        timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
-- organizations itself: NO RLS. It must be readable pre-context to RESOLVE the
-- tenant (docs/05 §4); access is mediated by membership joins in queries.
CREATE TRIGGER organizations_set_updated_at BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE slug_history (
  old_slug   citext PRIMARY KEY,
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL                 -- 30d 301 window (FR-TEN-007)
);

CREATE TABLE memberships (                         -- [T]
  org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       text NOT NULL CHECK (role IN ('OWNER','ADMIN','MEMBER','GUEST')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);  -- "my orgs" lookup

CREATE TABLE invitations (                         -- [T]
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
  UNIQUE (org_id, email)                           -- one pending per address
);

-- app_current_user_orgs — the sanctioned cross-tenant read for "my orgs".
-- memberships is RLS-isolated per tenant, so a plain user_id scan under the app
-- role returns nothing (no tenant GUC set). This SECURITY DEFINER function runs
-- as the owner (which is not FORCEd under RLS — see 0004) so it can list every
-- org a user belongs to, but ONLY that narrow, safe projection. See ADR-014.
CREATE FUNCTION app_current_user_orgs(p_user uuid)
RETURNS TABLE (org_id uuid, role text, slug citext, name text, created_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT m.org_id, m.role, o.slug, o.name, m.created_at
  FROM memberships m
  JOIN organizations o ON o.id = m.org_id
  WHERE m.user_id = p_user AND o.deleted_at IS NULL
  ORDER BY m.created_at;
$$;
REVOKE ALL ON FUNCTION app_current_user_orgs(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_current_user_orgs(uuid) TO fluxboard_app;

-- app_invitation_by_token — sanctioned cross-tenant read for the accept flow.
-- The invitee presents an opaque token whose SHA-256 is globally unique, but
-- the org is unknown until the row is read, and invitations is RLS-isolated.
-- This resolves the org (and invite fields) so the usecase can then open a
-- tenant-scoped transaction to bind membership + consume the invite. Returns at
-- most one row (token_hash is UNIQUE). See ADR-014.
CREATE FUNCTION app_invitation_by_token(p_hash bytea)
RETURNS TABLE (
  id uuid, org_id uuid, email citext, role text,
  invited_by uuid, expires_at timestamptz,
  accepted_at timestamptz, revoked_at timestamptz
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT i.id, i.org_id, i.email, i.role,
         i.invited_by, i.expires_at, i.accepted_at, i.revoked_at
  FROM invitations i
  WHERE i.token_hash = p_hash;
$$;
REVOKE ALL ON FUNCTION app_invitation_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_invitation_by_token(bytea) TO fluxboard_app;
