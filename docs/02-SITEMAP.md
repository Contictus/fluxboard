# 02 — Sitemap (Complete Page Inventory)

Next.js 14 App Router. Route groups: `(public)`, `(auth)`, `(app)`, `(admin)`.
Guard legend: **P** public · **A** authenticated · **V** authenticated+verified ·
**O(role)** org member with minimum role · **PA** platform admin.

## 1. Public / Marketing — `(public)`

| URL | Guard | Page |
|-----|-------|------|
| `/` | P | Landing page (hero, feature sections, social proof, CTA) |
| `/pricing` | P | Plan comparison table, FAQ, metered-billing explainer |
| `/features` | P | Feature overview |
| `/changelog` | P | Product changelog (MDX content) |
| `/legal/terms` | P | Terms of service |
| `/legal/privacy` | P | Privacy policy |
| `/legal/dpa` | P | Data processing addendum |
| `/status` | P | Uptime/status placeholder (links to status provider) |

## 2. Auth — `(auth)` (layout: centered card, no app chrome)

| URL | Guard | Page |
|-----|-------|------|
| `/login` | P | Email+password login, "continue with Google", link to register/forgot |
| `/login/2fa` | P* | TOTP challenge step (pending-auth token required) |
| `/register` | P | Registration form |
| `/verify-email` | A | Blocking "check your inbox" screen + resend (rate-limited) |
| `/verify-email/confirm?token=…` | P | Token consumption → success/expired states |
| `/forgot-password` | P | Request reset (identical response for unknown email) |
| `/reset-password?token=…` | P | Set new password |
| `/oauth/google/callback` | P | OAuth code exchange (server-side route handler, then redirect) |
| `/invite/{token}` | P | Invitation landing: logged-in → accept; logged-out → login/register then auto-accept |
| `/logout` | A | POST-only route handler; revokes session, clears cookie, redirects `/login` |

## 3. Account (user-scoped, org-independent) — `(app)`

| URL | Guard | Page |
|-----|-------|------|
| `/app` | V | Org switcher / "select or create organization" hub; auto-redirects if exactly 1 org |
| `/app/new-organization` | V | Create org form (name, slug with live availability check) |
| `/account/profile` | V | Name, avatar (MinIO upload), email change flow |
| `/account/security` | V | Change password, TOTP enroll/disable, recovery codes |
| `/account/sessions` | V | Active session list + revoke buttons + "log out all" |
| `/account/notifications` | V | Notification preference matrix (FR-NTF-004) |
| `/account/danger` | V | Delete account (blocked while sole OWNER of any org — lists them) |

## 4. Organization App — `(app)` under `/app/{orgSlug}` (layout: sidebar + topbar + org switcher)

| URL | Guard | Page |
|-----|-------|------|
| `/app/{orgSlug}` | O(GUEST) | Org home: assigned-to-me, recent activity, pinned projects |
| `/app/{orgSlug}/projects` | O(GUEST) | Project list/grid, filters (active/archived), create button |
| `/app/{orgSlug}/projects/new` | O(MEMBER) | Create project (name, key, visibility, template) |
| `/app/{orgSlug}/projects/{projectKey}` | O(GUEST)* | **Kanban board** (drag-drop, filters, bulk select, WIP indicators) |
| `/app/{orgSlug}/projects/{projectKey}/list` | O(GUEST)* | Same data as table view (sortable columns) |
| `/app/{orgSlug}/projects/{projectKey}/tasks/{taskNumber}` | O(GUEST)* | Task detail — intercepted route: modal over board, full page on direct load |
| `/app/{orgSlug}/projects/{projectKey}/analytics` | O(MEMBER) | Project analytics (FR-AN-001) |
| `/app/{orgSlug}/projects/{projectKey}/settings` | LEAD | Project settings: general, columns (reorder/WIP), members, danger (archive/delete) |
| `/app/{orgSlug}/my-tasks` | O(GUEST) | Cross-project assigned-to-me with grouping |
| `/app/{orgSlug}/search?q=…` | O(GUEST) | Full-text search results (tasks), filter sidebar |
| `/app/{orgSlug}/notifications` | O(GUEST) | Notification center (unread/all tabs) |
| `/app/{orgSlug}/trash` | O(MEMBER) | Soft-deleted tasks, restore/purge |

\* private projects additionally require project membership (403 → styled "no access" page).

## 5. Organization Settings — `/app/{orgSlug}/settings`

| URL | Guard | Page |
|-----|-------|------|
| `/app/{orgSlug}/settings` | O(ADMIN) | General: name, slug (with redirect warning), logo |
| `/app/{orgSlug}/settings/members` | O(ADMIN) | Member table: role dropdowns, remove, pending invitations tab (revoke/resend) |
| `/app/{orgSlug}/settings/members/invite` | O(ADMIN) | Invite form (multi-email, role) — blocks over seat limit with upgrade CTA |
| `/app/{orgSlug}/settings/labels` | O(MEMBER) | Org label CRUD |
| `/app/{orgSlug}/settings/api-keys` | O(ADMIN) | API key list, create dialog (one-time plaintext reveal), revoke |
| `/app/{orgSlug}/settings/audit-log` | O(ADMIN) | Org audit viewer + CSV export |
| `/app/{orgSlug}/settings/danger` | OWNER | Ownership transfer, org soft-delete flow |

## 6. Billing — `/app/{orgSlug}/billing`

| URL | Guard | Page |
|-----|-------|------|
| `/app/{orgSlug}/billing` | O(ADMIN) | Overview: plan card, seats, renewal, payment method (last4), `past_due` banner |
| `/app/{orgSlug}/billing/plans` | O(ADMIN) | Plan comparison, upgrade/downgrade buttons, proration preview dialog |
| `/app/{orgSlug}/billing/usage` | O(ADMIN) | Usage dashboard: meters vs limits, 30d charts, invoice estimate (FR-AN-002) |
| `/app/{orgSlug}/billing/invoices` | O(ADMIN) | Invoice history table (status, amount, PDF link) |
| `/app/{orgSlug}/billing/checkout/success?session_id=…` | O(ADMIN) | Post-checkout: polls subscription state ("finalizing…") until webhook lands |
| `/app/{orgSlug}/billing/checkout/cancelled` | O(ADMIN) | Checkout abandoned page |
| — | | "Manage payment method" button → server-created **Stripe Billing Portal** session (external redirect, no local page) |

## 7. Platform Admin — `(admin)` under `/admin` (separate layout, dark chrome)

| URL | Guard | Page |
|-----|-------|------|
| `/admin` | PA | KPIs: tenants, MRR, signups (30d), failed webhooks, error rate |
| `/admin/tenants` | PA | Tenant table: search, plan/status filters |
| `/admin/tenants/{orgId}` | PA | Tenant detail: subscription timeline, members, entitlement overrides, impersonate button |
| `/admin/users` | PA | User search (email), sessions, lock/unlock account |
| `/admin/webhooks` | PA | Stripe event browser: status, payload viewer, retry (FR-ADM-004) |
| `/admin/audit-log` | PA | Global audit viewer |
| `/admin/flags` | PA | Feature flag matrix per org |
| `/admin/jobs` | PA | Asynq queue stats (embed asynqmon or custom summary) |

## 8. System / Non-HTML Routes (Go API serves these; listed for completeness)

| URL | Purpose |
|-----|---------|
| `/api/v1/**` | REST API (08-API-SPEC.md) |
| `/api/v1/orgs/{orgId}/events` | SSE stream |
| `/api/v1/webhooks/stripe` | Stripe webhook receiver (signature-gated, no session auth) |
| `/api/v1/openapi.json`, `/api/docs` | OpenAPI + Swagger UI (non-prod) |
| `/healthz`, `/readyz` | Liveness (process up) / readiness (DB+Redis ping) |
| `/metrics` | Prometheus exposition (internal network only) |

## 9. Error & Edge Pages

| URL / trigger | Page |
|---|---|
| 404 anywhere | Styled not-found (app-shell variant inside `(app)`) |
| 403 in `(app)` | "You don't have access" + request-access hint |
| Org `past_due` | Persistent dismissable banner on all org pages (OWNER/ADMIN) |
| Org soft-deleted grace | Full-screen restore/countdown page replacing org routes (OWNER) |
| Plan limit hit (402 from API) | Contextual upgrade modal (shared component, reads `limit` field) |
| Maintenance flag | Global maintenance page (checked in root layout) |

**Page count:** ~55 distinct routes/screens (excluding modals-over-routes counted once).
