-- Runs once on first cluster init (mounted into /docker-entrypoint-initdb.d),
-- as the POSTGRES_USER superuser, against the POSTGRES_DB (fluxboard).
-- Creates the three roles from docs/10-INFRA-DEVOPS.md §1:
--   fluxboard_owner    — owns schema objects; migrations run as this role (DDL).
--   fluxboard_app      — runtime queries; SUBJECT to RLS (NOBYPASSRLS).
--   fluxboard_admin_ro — read-only analytics/admin.
-- Passwords here are LOCAL DEV ONLY and must match .env DSNs. Never used in prod.

CREATE ROLE fluxboard_owner    LOGIN PASSWORD 'owner_pw';
CREATE ROLE fluxboard_app      LOGIN PASSWORD 'app_pw'   NOSUPERUSER NOBYPASSRLS;
CREATE ROLE fluxboard_admin_ro LOGIN PASSWORD 'ro_pw'    NOSUPERUSER NOBYPASSRLS;

-- Owner gets CREATE on the database so migrations can install trusted
-- extensions (citext, pgcrypto) and create objects without superuser.
GRANT ALL ON DATABASE fluxboard TO fluxboard_owner;
ALTER SCHEMA public OWNER TO fluxboard_owner;

GRANT USAGE ON SCHEMA public TO fluxboard_app, fluxboard_admin_ro;

-- Objects created later by the owner are automatically usable by app (DML) and
-- admin_ro (SELECT), so no per-migration GRANT boilerplate is needed.
ALTER DEFAULT PRIVILEGES FOR ROLE fluxboard_owner IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO fluxboard_app;
ALTER DEFAULT PRIVILEGES FOR ROLE fluxboard_owner IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO fluxboard_app;
ALTER DEFAULT PRIVILEGES FOR ROLE fluxboard_owner IN SCHEMA public
  GRANT SELECT ON TABLES TO fluxboard_admin_ro;
