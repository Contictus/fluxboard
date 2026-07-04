-- 0001_extensions — enable the extensions every later migration relies on.
-- No tables yet: schema tables arrive in Phase 1+ per docs/CLAUDE.md build order.
--
-- citext    : case-insensitive email columns (users.email — docs/07 §1).
-- pgcrypto  : gen_random_bytes / digest helpers used by later migrations.
--
-- RLS convention (docs/05-TENANCY-RBAC.md §4): every tenant-owned table added
-- later enables ROW LEVEL SECURITY and filters on the per-connection GUC
-- current_setting('app.current_tenant'). The tenant pool sets it via
-- SET LOCAL on each checked-out connection. Nothing to enable here yet — this
-- comment is the anchor so the pattern is documented from migration one.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
