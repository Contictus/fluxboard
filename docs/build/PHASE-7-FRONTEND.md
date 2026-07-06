# Phase 7 — Frontend (Next.js 14, wired to the API)

**Scope:** every route in `docs/02-SITEMAP.md` (~55 screens) · **Spec:**
`docs/02-SITEMAP.md`, `docs/CLAUDE.md` §TypeScript/Next.js. **Depends on:**
Phases 4–6 backend done (an area can start once its backing API exists).
**Gate:** all sitemap pages wired to the API; typecheck + lint green.

> Sections are per sitemap area, not the backend section shape. Conventions
> (docs/CLAUDE.md): App Router, TS strict (no `any`), Server Components by
> default (`"use client"` only where needed), all mutations via TanStack Query
> with optimistic updates where specified, access token in memory + refresh in
> httpOnly Secure SameSite=Strict cookie (NEVER localStorage), API types
> generated from OpenAPI (`make gen-client`).

Verification per area = the page renders and its happy path works against the
running dockerized API. Frontend commit gate = `pnpm typecheck && pnpm lint &&
pnpm build` green (no `go test`).

---

## Section 0 — Scaffolding & foundations

- [ ] 7.0.1 Scaffold `web/` — Next.js 14 App Router, TypeScript strict, ESLint/Prettier; add `web` compose service + Makefile `web` target.
- [ ] 7.0.2 Tailwind + shadcn/ui base (theme, components dir).
- [ ] 7.0.3 TanStack Query v5 provider; generate typed API client from `/api/v1/openapi.json` (`make gen-client`, depends on Phase 6 §6.4.5).
- [ ] 7.0.4 Auth context: access token in memory, refresh via httpOnly cookie; silent-refresh on 401; logout clears.
- [ ] 7.0.5 Route groups `(public)`/`(auth)`/`(app)`/`(admin)` + guard helpers (P/A/V/O(role)/PA) per sitemap legend.
- [ ] 7.0.6 Root layout: maintenance-flag check, global 404/403 boundaries, 402 upgrade-modal mount point.

## Section 1 — Public / Marketing `(public)`

- [ ] 7.1.1 `/` landing, `/pricing` (plan table + metered explainer), `/features`.
- [ ] 7.1.2 `/changelog` (MDX), `/legal/{terms,privacy,dpa}`, `/status`.

## Section 2 — Auth `(auth)`

- [ ] 7.2.1 `/login` — email+password + "continue with Google" + links. FR-AUTH-003/009.
- [ ] 7.2.2 `/login/2fa` — TOTP challenge (pending-auth token). FR-AUTH-013.
- [ ] 7.2.3 `/register`. FR-AUTH-001.
- [ ] 7.2.4 `/verify-email` (blocking + resend rate-limited) + `/verify-email/confirm?token=`. FR-AUTH-002.
- [ ] 7.2.5 `/forgot-password` (identical response) + `/reset-password?token=`. FR-AUTH-010.
- [ ] 7.2.6 `/oauth/google/callback` (route handler), `/invite/{token}` (logged-in accept / logged-out login-then-accept), `/logout`. FR-AUTH-009, FR-TEN-004.

## Section 3 — Account (user-scoped) `(app)`

- [ ] 7.3.1 `/app` org switcher (auto-redirect if exactly 1) + `/app/new-organization` (slug live availability). FR-TEN-001/002.
- [ ] 7.3.2 `/account/profile` — name, avatar MinIO upload, email-change flow. FR-AUTH-014.
- [ ] 7.3.3 `/account/security` (password, TOTP enroll/disable, recovery codes) + `/account/sessions` (list + revoke + log-out-all). FR-AUTH-008/012/013.
- [ ] 7.3.4 `/account/notifications` prefs matrix (FR-NTF-004) + `/account/danger` (delete, blocked while sole OWNER).

## Section 4 — Org shell + home

- [ ] 7.4.1 `/app/{orgSlug}` layout — sidebar + topbar + org switcher; role-aware nav.
- [ ] 7.4.2 Org home: assigned-to-me, recent activity, pinned projects. FR (org home).
- [ ] 7.4.3 Edge states: `past_due` dismissable banner (OWNER/ADMIN); soft-delete grace full-screen restore/countdown. 02 §9.

## Section 5 — Projects + Kanban board

- [ ] 7.5.1 `/projects` list/grid + active/archived filters + create button. FR-PROJ-001/006.
- [ ] 7.5.2 `/projects/new` (name, key, visibility, template). FR-PROJ-001/002.
- [ ] 7.5.3 `/projects/{projectKey}` Kanban board — dnd-kit columns + cards, WIP indicators. FR-PROJ-004/005.
- [ ] 7.5.4 Drag-drop → LexoRank move with optimistic update + rollback on 409. FR-PROJ-005.
- [ ] 7.5.5 Board filters + bulk multi-select actions (move/assign/label). FR-TASK-008.
- [ ] 7.5.6 `/projects/{projectKey}/list` table view + `/settings` (general, columns reorder/WIP, members, archive/delete). FR-PROJ-003/004/006.

## Section 6 — Task detail + search/trash/my-tasks

- [ ] 7.6.1 `/projects/{projectKey}/tasks/{taskNumber}` — intercepted route (modal over board, full page on direct load). 02 §4.
- [ ] 7.6.2 Inline edit of every field + activity timeline. FR-TASK-001/002.
- [ ] 7.6.3 Subtasks checklist + comments (markdown, 15-min edit marker, @mention autocomplete of project members). FR-TASK-003/005.
- [ ] 7.6.4 Attachments UI — request-upload → presigned PUT (direct to MinIO) → confirm; download via presigned GET. FR-TASK-006.
- [ ] 7.6.5 `/search?q=` results + filter sidebar, `/my-tasks`, `/trash` (restore/purge). FR-TASK-007/009.

## Section 7 — Org settings

- [ ] 7.7.1 `/settings` general (name, slug with redirect warning, logo). FR-TEN-007.
- [ ] 7.7.2 `/settings/members` (role dropdowns, remove, pending-invites tab) + `/members/invite` (multi-email, seat-limit CTA on 402). FR-TEN-004/005.
- [ ] 7.7.3 `/settings/labels` CRUD. FR-TASK-004.
- [ ] 7.7.4 `/settings/api-keys` (create dialog one-time reveal, revoke), `/settings/audit-log` (+CSV), `/settings/danger` (ownership transfer, soft-delete). FR-API-001, FR-AUD-003, FR-TEN-008.

## Section 8 — Billing

- [ ] 7.8.1 `/billing` overview (plan card, seats, renewal, payment method last4, past_due banner). FR-BILL-001.
- [ ] 7.8.2 `/billing/plans` (upgrade/downgrade, proration-preview dialog). FR-BILL-002/003.
- [ ] 7.8.3 Checkout redirect → `/billing/checkout/success` polls summary until webhook lands ("finalizing…", 60s timeout) + `/checkout/cancelled`. FR-BILL-002, 06 §3.
- [ ] 7.8.4 `/billing/usage` dashboard (Recharts meters/30d charts, invoice estimate, FR-AN-002) + `/billing/invoices` (status/amount/PDF) + "manage payment method" → Billing Portal redirect. FR-BILL-006/008.
- [ ] 7.8.5 Shared 402 upgrade modal (reads `limit` field) mounted app-wide. FR-BILL-009, 02 §9.

## Section 9 — Notifications + SSE client

- [ ] 7.9.1 `web/lib/sse.ts` — EventSource wrapper: reconnect backoff 1→2→5→10s +jitter, `Last-Event-ID`, `resync` handling. FR-NTF-001, 09 §1.
- [ ] 7.9.2 Per-event surgical TanStack cache updates (e.g. `task.moved` moves the card, not blanket invalidate); skip `actor_id === me`.
- [ ] 7.9.3 `/notifications` center (unread/all tabs, mark read/all-read) + topbar unread badge. FR-NTF-002.

## Section 10 — Platform admin `(admin)` + analytics

- [ ] 7.10.1 `/admin` route group (separate dark layout, PA guard) + KPI dashboard. FR-ADM-001.
- [ ] 7.10.2 `/admin/tenants` + `/admin/tenants/{orgId}` (subscription timeline, overrides, impersonate button) + `/admin/users`. FR-ADM-002/003.
- [ ] 7.10.3 `/admin/webhooks` (payload viewer + retry), `/admin/audit-log`, `/admin/flags`, `/admin/jobs`. FR-ADM-004/005/006.
- [ ] 7.10.4 Impersonation banner (read-only session) shown app-wide when `imp` active. FR-ADM-003.
- [ ] 7.10.5 `/projects/{projectKey}/analytics` project analytics (Recharts: completed/week, cumulative flow, cycle time, per-assignee). FR-AN-001.

## Section 11 — Verify & commit

- [ ] 7.11.1 [VERIFY] Every sitemap route reachable + guards enforced against the running API; core flows (login→org→board→task→billing) work end-to-end.
- [ ] 7.11.2 [VERIFY] `pnpm typecheck && pnpm lint && pnpm build` green; no `any`.
- [ ] 7.11.3 `git commit` (`feat(web): phase 7 — frontend wired to API`); mark README build COMPLETE.

---

## Definition of Done (Phase 7)

- [ ] All ~55 sitemap routes implemented + guarded per legend.
- [ ] Board drag-drop optimistic + 409 rollback; SSE surgical cache updates.
- [ ] Checkout success-poll, 402 upgrade modal, past_due/soft-delete edge pages.
- [ ] Admin route group isolated + impersonation banner.
- [ ] typecheck/lint/build green; committed. **Build complete** — update README.
