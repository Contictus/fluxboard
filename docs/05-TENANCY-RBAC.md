# 05 — Multi-Tenancy & Authorization

Implements FR-TEN-*, plus the authorization layer used by every module.

## 1. Tenancy Model

Shared database, shared schema, `org_id UUID NOT NULL` discriminator on every
tenant-owned table, enforced by **two independent layers**:

1. **Application layer:** every sqlc query on tenant tables includes
   `org_id = $1` (reviewed in PR; grep-lint in CI rejects tenant-table
   queries without `org_id` in the WHERE clause).
2. **Database layer (backstop):** PostgreSQL Row-Level Security keyed on a
   per-transaction GUC.

Rationale vs schema-per-tenant / database-per-tenant: ADR-004 (03 §6).

## 2. Role Model

**Org roles** (membership.role): `OWNER > ADMIN > MEMBER > GUEST`.
**Project roles** (project_members.role): `LEAD > CONTRIBUTOR > VIEWER`.
Effective project access = max(implicit-from-org, explicit-project-role):
org ADMIN+ ⇒ implicit LEAD everywhere; org MEMBER ⇒ implicit CONTRIBUTOR on
`org`-visibility projects, nothing on `private` ones; GUEST ⇒ explicit
project membership only.

Permission matrix (authoritative; Casbin policies mirror this):

| Action | OWNER | ADMIN | MEMBER | GUEST |
|---|---|---|---|---|
| org: rename/logo | ✓ | ✓ | – | – |
| org: slug change, delete, ownership transfer | ✓ | – | – | – |
| billing: view/manage | ✓ | ✓ | – | – |
| members: invite/remove/change role | ✓ | ✓* | – | – |
| api-keys: manage | ✓ | ✓ | – | – |
| audit log: view org | ✓ | ✓ | – | – |
| project: create | ✓ | ✓ | ✓ | – |
| project: settings/archive/delete | LEAD or ADMIN+ | | | – |
| task: create/edit/move | project CONTRIBUTOR+ | | | per-project |
| task: view | per project visibility | | | per-project |
| comment: create | project CONTRIBUTOR+ | | | per-project |
| comment: edit/delete own | author (15 min edit window) | | | |
| labels: manage | ✓ | ✓ | ✓ | – |

\* ADMIN cannot modify/remove OWNER or grant OWNER.

## 3. Casbin Model

`internal/infrastructure/casbinx/model.conf`:

```ini
[request_definition]
r = sub, dom, obj, act        # sub=user_id, dom=org_id, obj=resource, act=action

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _                   # user has role in domain (org)

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && \
    keyMatch2(r.obj, p.obj) && r.act == p.act
```

- Grouping facts written on membership change:
  `g, user:{id}, role:ADMIN, org:{org_id}` (and removed on role change/removal —
  membership usecase and Casbin adapter update in the same transaction).
- Static role policies seeded per org creation from a template
  (`p, role:ADMIN, org:{id}, /projects/*, write` …).
- **Project-level and ownership checks** (project membership, comment author,
  private visibility) are NOT modeled as Casbin policies per task — they are
  domain checks in the usecase layer (`taskuc.canEdit(ctx, task)`), because
  per-resource policies would explode the policy table. Casbin answers
  "org-role coarse gate"; usecase answers "resource fine gate". Both must pass.

Middleware usage — routes declare their object/action:

```go
r.With(mw.Authorize("/projects/*", "write")).Post("/projects", h.CreateProject)
```

## 4. RLS Implementation (the critical piece)

Postgres roles: migrations run as `fluxboard_owner`; the app connects as
`fluxboard_app` which does **not** have `BYPASSRLS`.

Policy pattern applied to every tenant table:

```sql
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks FORCE ROW LEVEL SECURITY;   -- applies to table owner too

CREATE POLICY tenant_isolation ON tasks
  USING (org_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (org_id = current_setting('app.current_tenant', true)::uuid);
```

`current_setting(..., true)` returns NULL when the GUC is unset ⇒ predicate
is NULL ⇒ **zero rows**. Fail-closed: a request that forgot tenant context
reads nothing rather than everything.

Connection wrapper (`tenantpool.go`) — the ONLY sanctioned way usecases touch
tenant data:

```go
func (p *TenantPool) WithTenant(ctx context.Context, orgID uuid.UUID,
    fn func(q *gen.Queries) error) error {
    return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
        // SET LOCAL: scoped to this transaction; safe with pgx pooling because
        // the GUC dies with the tx — no leakage across pooled connections.
        if _, err := tx.Exec(ctx,
            "SELECT set_config('app.current_tenant', $1, true)",
            orgID.String()); err != nil {
            return err
        }
        return fn(gen.New(tx))
    })
}
```

Rules:
- `SET LOCAL` (transaction-scoped), never session-scoped `SET` — pooled
  connections would leak tenant context otherwise. This is the classic
  RLS-with-pooling footgun; the wrapper makes it unreachable.
- Non-tenant tables (users, sessions, processed_stripe_events, plans) have no
  RLS and are accessed via a separate `GlobalPool` — the type system keeps
  the two pools distinct so a tenant query cannot compile against the global
  pool's Queries struct by accident (separate sqlc packages).
- Platform-admin cross-tenant reads use a third, audited path
  (`AdminPool`, connects as `fluxboard_admin_ro`, read-only role) — never by
  bypassing the wrapper with the app role.

## 5. Invitation Flow (FR-TEN-004) — sequence

```
ADMIN → POST /orgs/{id}/invitations {email, role}
  ├─ entitlement: seat count + pending invites < plan limit, else 402
  ├─ reject if already member / already pending (409)
  ├─ insert invitation (token hash, expires 7d) + audit + enqueue email
Invitee clicks /invite/{token}
  ├─ logged-in, email matches      → accept: membership insert + Casbin g-fact
  │                                   + invitation consumed (single tx) + notify inviter
  ├─ logged-in, email differs      → warning screen (accept anyway = allowed,
  │                                   membership binds to the logged-in account)
  └─ logged-out                    → login/register with email prefilled → auto-accept
```

## 6. Failure-Mode Tests (gate for Phase 2 — full list 12 §6)

- Direct object reference across tenants (task UUID of org B via org A context) → 404
- Query with tenant GUC unset (deliberately broken test harness) → 0 rows, not all rows
- Org ADMIN of A calls admin endpoints of B → 403 (Casbin domain mismatch)
- Last-OWNER demote/leave → 409 `last_owner`
- Removed member's live SSE connection receives `membership.revoked` and subsequent API calls 403 within cache TTL (≤ 60 s)
