# 01 — Functional Requirements (Exhaustive)

Requirement ID format: `FR-<MODULE>-<NNN>`. Every requirement is testable.
Priority: **M** = must (v1 blocker), **S** = should, **C** = could.
Acceptance criteria for each phase = all **M** requirements of that module pass.

---

## AUTH — Authentication & Session Management

| ID | P | Requirement |
|----|---|-------------|
| FR-AUTH-001 | M | User can register with email + password. Password policy: ≥ 12 chars, checked against a local top-10k breached-password list. Hash: Argon2id (memory=64MB, iterations=3, parallelism=2, salt 16B, key 32B). |
| FR-AUTH-002 | M | Registration sends a verification email containing a single-use token (32B random, SHA-256 hash stored, 24h TTL). Unverified users can log in but see a blocking "verify your email" screen; all other app routes redirect there. |
| FR-AUTH-003 | M | User can log in with email + password. Response sets: access token (JWT, 15 min TTL, returned in JSON body) + refresh token (opaque 32B random, httpOnly Secure SameSite=Strict cookie, 7 day TTL). |
| FR-AUTH-004 | M | Access JWT claims: `sub` (user_id), `sid` (session_id), `iat`, `exp`, `iss=fluxboard`, `aud=fluxboard-api`. Signed ES256. NO role/tenant claims in the token — authorization is resolved per-request server-side (roles change without waiting for token expiry). |
| FR-AUTH-005 | M | Refresh endpoint rotates the refresh token: old token is marked `rotated_at`, new token issued, both linked by `family_id`. |
| FR-AUTH-006 | M | **Reuse detection:** presenting an already-rotated refresh token revokes the ENTIRE family (all sessions descended from the same login) and returns 401. Event logged to audit log with severity=security. |
| FR-AUTH-007 | M | Logout revokes the current session (refresh token family) and clears the cookie. "Log out all devices" revokes every session of the user. |
| FR-AUTH-008 | M | User can view active sessions (device/user-agent, IP, last-used, created) and revoke any individually. |
| FR-AUTH-009 | M | Google OAuth 2.0 login via Authorization Code + PKCE (S256). `state` parameter (32B random, 10 min TTL in Redis) validated for CSRF. On first OAuth login: if email matches an existing verified account → link identity; else create user with `email_verified=true`. |
| FR-AUTH-010 | M | Password reset: request → email with single-use token (1h TTL, hash stored) → set new password → ALL sessions revoked. Response to the request endpoint is identical whether or not the email exists (no user enumeration). |
| FR-AUTH-011 | M | Login endpoint rate limit: 5 failures / 15 min per (email, IP) pair → 429 + `Retry-After`. Backed by Redis sliding window. |
| FR-AUTH-012 | M | Change password (while logged in) requires current password; revokes all OTHER sessions. |
| FR-AUTH-013 | S | TOTP 2FA: enroll (QR + secret), verify, 10 single-use recovery codes (hashes stored). When enabled, login becomes two-step: password → TOTP challenge (5 min pending-auth token). |
| FR-AUTH-014 | S | Change email: verification sent to NEW address; old address gets a notification with revert link (72h). |
| FR-AUTH-015 | C | WebAuthn/passkey login. |

## TEN — Tenancy, Organizations, Membership

| ID | P | Requirement |
|----|---|-------------|
| FR-TEN-001 | M | Authenticated user can create an organization (name, unique slug `[a-z0-9-]{3,40}`). Creator becomes OWNER. Free plan auto-assigned; Stripe customer created lazily on first paid action. |
| FR-TEN-002 | M | A user can belong to multiple organizations; an org switcher lists them; the active org is part of the URL (`/app/{orgSlug}/…`), NOT stored server-side session state. |
| FR-TEN-003 | M | Org roles: OWNER, ADMIN, MEMBER, GUEST. Exactly ≥ 1 OWNER at all times (last OWNER cannot leave/demote; transfer required). Full permission matrix in 05-TENANCY-RBAC.md §2. |
| FR-TEN-004 | M | ADMIN+ can invite by email with a role. Invitation = single-use token (7 day TTL) emailed as a link. Invitee flow: existing user → accept while logged in; new user → register-then-accept in one flow. Pending invitations listable/revocable. Accepting counts against the plan's member limit; over-limit invites blocked at SEND time with an upgrade prompt. |
| FR-TEN-005 | M | ADMIN+ can change member roles (cannot touch OWNER unless self is OWNER) and remove members. Removal: user's task assignments become unassigned, comments/audit entries retained with authorship. |
| FR-TEN-006 | M | Member can leave an org (except last OWNER). |
| FR-TEN-007 | M | OWNER can rename org, change slug (old slug 301-redirects for 30 days via slug_history table), upload logo (MinIO). |
| FR-TEN-008 | M | OWNER can soft-delete the org: 14-day grace (banner + restore option), then hard purge by background job. Active paid subscription must be cancelled first (guided flow). |
| FR-TEN-009 | M | Every tenant-owned row carries `org_id`; RLS policies enforce isolation (05-TENANCY-RBAC.md §4). Cross-tenant test suite (12-TESTING.md §6) is the acceptance gate. |
| FR-TEN-010 | S | GUEST role: visibility only into explicitly-assigned projects; excluded from member-seat billing count. |

## PROJ — Projects & Boards

| ID | P | Requirement |
|----|---|-------------|
| FR-PROJ-001 | M | MEMBER+ can create a project: name, key (2–6 uppercase letters, unique per org, e.g. `PAY`), description (markdown), color. Plan project-limit enforced at creation. |
| FR-PROJ-002 | M | Project visibility: `org` (all org members) or `private` (only project members). GUEST sees only private projects they belong to. |
| FR-PROJ-003 | M | Project membership with project roles LEAD / CONTRIBUTOR / VIEWER; LEAD manages project settings & members. Org ADMIN+ implicitly has LEAD on all projects. |
| FR-PROJ-004 | M | Default board per project with columns (statuses): Backlog, Todo, In Progress, Done. LEAD can add/rename/reorder/delete columns (delete requires migrating its tasks to another column — UI prompts). Column WIP limit optional; exceeding shows a visual warning (not a hard block). |
| FR-PROJ-005 | M | Kanban board: drag-and-drop tasks within/between columns. Ordering uses **lexicographic rank keys** (LexoRank-style) so a move touches exactly one row. Optimistic UI with rollback on 409. |
| FR-PROJ-006 | M | Project archive (read-only, excluded from default lists) and unarchive. |
| FR-PROJ-007 | S | Project templates: create a project from a predefined column/label set. |

## TASK — Tasks, Subtasks, Comments, Attachments

| ID | P | Requirement |
|----|---|-------------|
| FR-TASK-001 | M | Create task: title (≤ 200), description (markdown), column, assignee (project member), priority (urgent/high/medium/low/none), due date, labels. Auto-numbered per project: `PAY-123` (sequence per project, gap-free not required). |
| FR-TASK-002 | M | Task detail: inline edit of every field; every change writes an activity entry (field, old→new, actor, timestamp). |
| FR-TASK-003 | M | Subtasks: flat one-level checklist under a task (title + done). Progress (3/7) surfaces on the board card. |
| FR-TASK-004 | M | Labels: org-scoped (name ≤ 30, color). CRUD by MEMBER+; deleting a label detaches it everywhere. |
| FR-TASK-005 | M | Comments: markdown, edit within 15 min (edited marker), soft delete ("comment deleted" placeholder). @mention of project members triggers a notification (FR-NTF-002). |
| FR-TASK-006 | M | Attachments: upload via MinIO **presigned PUT** (client → MinIO directly, API never proxies bytes). Flow: request-upload (validates size ≤ 25 MB, MIME allowlist, storage quota) → presigned URL → client PUT → confirm-upload (HEAD verifies existence & size; row committed; usage counter incremented). Download via presigned GET (5 min TTL). Orphan uploads GC'd by nightly job. |
| FR-TASK-007 | M | Task search within org: title + description, PostgreSQL FTS (tsvector, GIN), filter by project/assignee/label/priority/status; paginated. |
| FR-TASK-008 | M | Bulk actions on board: multi-select → move column / assign / label (single transaction; single realtime event batch). |
| FR-TASK-009 | M | Soft-delete task → Trash (30 days) → restore or permanent purge by job. |
| FR-TASK-010 | S | Task relations: `blocks` / `blocked-by` links with cycle detection (reject with 422 + the detected path). |
| FR-TASK-011 | C | Recurring tasks. |

## BILL — Billing & Payments (full spec: 06-BILLING.md)

| ID | P | Requirement |
|----|---|-------------|
| FR-BILL-001 | M | OWNER/ADMIN sees billing page: current plan, seat count, renewal date, payment method (brand + last4 only), usage meters vs limits. |
| FR-BILL-002 | M | Upgrade Free→Pro/Business through Stripe Checkout (hosted). Checkout session created server-side with an **idempotency key**; success/cancel URLs return to the app; final state authority is the WEBHOOK, not the redirect. |
| FR-BILL-003 | M | Plan change Pro↔Business: proration handled by Stripe subscription update; preview of prorated amount shown before confirm (upcoming-invoice API). |
| FR-BILL-004 | M | Cancel: at period end (default, with resume option) — immediate cancel is platform-admin only. Downgrade-to-Free consequences (over-limit projects become read-only, members over cap blocked from write) enforced by entitlement middleware from period end. |
| FR-BILL-005 | M | Webhook consumer handles: `checkout.session.completed`, `customer.subscription.created/updated/deleted`, `invoice.payment_succeeded`, `invoice.payment_failed`, `invoice.finalized`. **Idempotent**: `processed_stripe_events(event_id UNIQUE)` inserted in the SAME transaction as all side effects; duplicate → 200 no-op. Signature verified (`Stripe-Signature`, 5 min tolerance). Out-of-order events tolerated by comparing `event.created` against the local subscription's `last_stripe_event_at`. |
| FR-BILL-006 | M | Dunning: `invoice.payment_failed` → org enters `past_due` (banner for OWNER/ADMIN + email). Stripe Smart Retries; after final failure → subscription `unpaid` → entitlements drop to Free. Payment-method update via Stripe Billing Portal session. |
| FR-BILL-007 | M | Usage metering (Business): Redis counters (active members daily HLL, storage bytes gauge, API calls counter) → hourly Asynq job aggregates into `usage_records` → daily job pushes to Stripe metered subscription items (`action=set`). Pipeline is idempotent per (org, metric, period). |
| FR-BILL-008 | M | Invoice history page: list from local mirror (synced via `invoice.finalized` webhook), link to Stripe-hosted invoice PDF. |
| FR-BILL-009 | M | Entitlement middleware: every write endpoint declares required entitlements; middleware resolves org plan+status (Redis-cached 60s, invalidated by webhook consumer) and returns 402 `plan_limit_exceeded` with a machine-readable `limit` field when exceeded. |
| FR-BILL-010 | S | Trial: 14-day Business trial without card; countdown banner; auto-downgrade to Free on expiry. |
| FR-BILL-011 | C | Promo codes via Stripe coupons at checkout. |

## NTF — Notifications & Realtime

| ID | P | Requirement |
|----|---|-------------|
| FR-NTF-001 | M | Realtime channel: SSE endpoint `GET /api/v1/orgs/{orgId}/events` (auth'd). Events: task.created/updated/moved/deleted, comment.created, member.joined, notification.created. Client auto-reconnects with `Last-Event-ID`; server replays ≤ 5 min from a Redis Stream per org. |
| FR-NTF-002 | M | In-app notifications for: task assigned to you, @mention, comment on a task you're assigned to/created, invitation accepted, billing events (OWNER/ADMIN). Unread badge; mark read/all-read; 90-day retention. |
| FR-NTF-003 | M | Email notifications (Asynq → SMTP; Mailpit locally): invitation, verification, password reset, payment failed, weekly digest (S). Per-user per-category opt-out (transactional auth emails cannot be disabled). |
| FR-NTF-004 | S | Notification preferences page (matrix: category × channel). |

## ADM — Platform Admin

| ID | P | Requirement |
|----|---|-------------|
| FR-ADM-001 | M | Separate admin surface `/admin` (separate Next.js route group + separate Go router). Access: users with `platform_role=admin` + mandatory TOTP. Not reachable through tenant middleware. |
| FR-ADM-002 | M | Tenant list: search, plan, status, member count, MRR; tenant detail: subscription state, recent webhook events for that customer, entitlement overrides. |
| FR-ADM-003 | M | **Impersonation:** admin can open a read-only impersonated session into an org (dedicated short-lived token, `imp` claim). Banner shown; every impersonated request audit-logged with both identities; writes are rejected. |
| FR-ADM-004 | M | Webhook event browser: raw Stripe events, processing status, error, retry button (re-runs consumer through the same idempotency path). |
| FR-ADM-005 | M | Global audit log viewer with filters (org, actor, action, severity, date range). |
| FR-ADM-006 | S | Feature flags per org (simple DB-backed flags exposed to entitlement middleware). |

## AUD — Audit Log

| ID | P | Requirement |
|----|---|-------------|
| FR-AUD-001 | M | Append-only `audit_log` for: all auth events (login success/fail, refresh reuse, password/email change, session revoke), membership & role changes, project create/archive/delete, billing state transitions, admin/impersonation actions. Fields: id, org_id (nullable for platform events), actor_user_id, impersonator_user_id, action, target_type, target_id, metadata jsonb, ip, user_agent, severity, created_at. |
| FR-AUD-002 | M | No UPDATE/DELETE grants on audit_log for the app role; retention purge (per plan tier) runs as a separate privileged job. |
| FR-AUD-003 | M | Org-scoped audit viewer for OWNER/ADMIN (filter by actor/action/date, CSV export ≤ 10k rows). |

## API — Public API & Keys

| ID | P | Requirement |
|----|---|-------------|
| FR-API-001 | M | ADMIN+ can create org-scoped API keys: prefix `fbk_live_` + 32B random; SHA-256 hash stored; plaintext shown ONCE. Scopes: `read`, `write`. Revocable; last-used timestamp tracked. |
| FR-API-002 | M | API-key auth path (`Authorization: Bearer fbk_…`) resolves org context from the key (no user session). Rate limited per key by plan tier (Redis sliding window; headers `X-RateLimit-Limit/Remaining/Reset`; 429 + Retry-After). |
| FR-API-003 | M | OpenAPI 3.1 spec generated from code annotations; served at `/api/v1/openapi.json`; Swagger UI on `/api/docs` (non-prod). Frontend client types generated from this spec. |
| FR-API-004 | M | API calls counter per org per day (Redis) feeds usage metering (FR-BILL-007) and the usage dashboard. |

## AN — Analytics (in-app)

| ID | P | Requirement |
|----|---|-------------|
| FR-AN-001 | M | Project analytics page: tasks completed per week (12w bar), cumulative flow by column (stacked area), avg cycle time (In Progress→Done, 30d), per-assignee open/closed counts. Powered by nightly rollup table `project_stats_daily` — NOT live aggregate queries on the task table. |
| FR-AN-002 | M | Org usage dashboard: seats vs limit, storage vs limit, API calls (30d sparkline), next-invoice estimate for metered plans (from local usage_records, labeled "estimate"). |
