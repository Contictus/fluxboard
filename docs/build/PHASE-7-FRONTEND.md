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

## Section 0 — Scaffolding & foundations  ✅ (commit `e64737a`)

- [x] 7.0.1 Scaffold `web/` — Next.js 14 App Router, TypeScript strict, ESLint/Prettier; add `web` compose service + Makefile `web` target.
- [x] 7.0.2 Tailwind + shadcn/ui base (theme tokens, hand-authored `components/ui` primitives).
- [x] 7.0.3 TanStack Query v5 provider; **hand-written typed fetch client** (`lib/api/client.ts`) instead of `make gen-client` — the served OpenAPI only covers Phase-6 handlers, so types are authored per docs/08 and grown per section.
- [x] 7.0.4 Auth context: access token in memory, cookie→refresh bootstrap on mount, single-flight silent-refresh on 401; logout clears + revokes.
- [x] 7.0.5 Route groups `(public)`/`(auth)`/`(app)`/`(admin)` + guard helpers (`RequireAuth`/`RequireVerified`/`RequirePlatformAdmin`). PA is session-only for now (platform_role comes from `/me`).
- [x] 7.0.6 Root layout: global 404/403 boundaries, 402 upgrade-modal mount point (body wired in Phase 8). Maintenance-flag check deferred to org shell.

## Section 1 — Public / Marketing `(public)`  ✅ (commit `aeaafbd`)

- [x] 7.1.1 `/` landing, `/pricing` (plan table + metered explainer + FAQ), `/features`.
- [x] 7.1.2 `/changelog` (content array), `/legal/{terms,privacy,dpa}`, `/status`.

## Section 2 — Auth `(auth)`  ✅ (commit `b9b7f55`)

- [x] 7.2.1 `/login` — email+password + "continue with Google" + links. FR-AUTH-003/009.
- [x] 7.2.2 `/login/2fa` — TOTP challenge (pending-auth token in sessionStorage). FR-AUTH-013.
- [x] 7.2.3 `/register`. FR-AUTH-001.
- [x] 7.2.4 `/verify-email` (blocking + resend rate-limited) + `/verify-email/confirm?token=`. FR-AUTH-002.
- [x] 7.2.5 `/forgot-password` (identical response) + `/reset-password?token=`. FR-AUTH-010.
- [x] 7.2.6 `/oauth/google/callback` (reads `#access_token` fragment), `/invite/{token}` (logged-in accept / logged-out login-then-accept), `/logout` (client-side; token is in memory). FR-AUTH-009, FR-TEN-004.

## Section 3 — Account (user-scoped) `(app)`  ✅ (backend §B `c6b45a4`, web commit below)

- [x] 7.3.1 `/app` org switcher + `/app/new-organization` (slug live availability). FR-TEN-001/002. NOTE: auto-redirect-if-1 deferred until the org shell (§4) exists so users aren't bounced to a 404.
- [x] 7.3.2 `/account/profile` — name (`PATCH /me`), avatar MinIO upload (presigned PUT→confirm). FR-AUTH-014. Email-change control disabled (deferred, ADR-015).
- [x] 7.3.3 `/account/security` (password change, TOTP enroll/activate/disable + recovery codes) + `/account/sessions` (list + revoke + log-out-all). FR-AUTH-008/012/013.
- [x] 7.3.4 `/account/notifications` prefs matrix — **per-org** with an org selector (global prefs not built; ADR-015) — + `/account/danger` (`DELETE /me`, 409 sole_owner_of block list). FR-NTF-004.

## Section 4 — Org shell + home  ✅ (commit below)

- [x] 7.4.1 `/app/{orgSlug}` layout — sidebar + topbar + org switcher; role-aware nav. Org context (`lib/org/context.tsx`) resolves slug via `GET /orgs` (role) + `GET /orgs/{id}` (deleted_at); unknown slug bounces to `/app`. Sidebar admin-only items (Members/Settings/Billing) link forward to §5–8 routes.
- [x] 7.4.2 Org home: assigned-to-me (`tasks/search?assignee_id=<me>`), recent activity (own `/notifications` — no org-wide feed, ADR-016), projects (recent active from `/projects` — no pin concept, ADR-016). FR (org home).
- [x] 7.4.3 Edge states: `past_due` dismissable banner — OWNER/ADMIN only, `billing/summary` is ADMIN+ gated (ADR-016); soft-delete grace full-screen restore CTA (OWNER restores; no live countdown — `purge_after` not exposed, ADR-016). 02 §9.

## Section 5 — Projects + Kanban board  ✅ (commit below)

- [x] 7.5.1 `/projects` list/grid + active/archived filters + create button. FR-PROJ-001/006. NOTE: archived filter reuses `?archived=true` (which includes actives) then filters to `archived_at`-set client-side.
- [x] 7.5.2 `/projects/new` (name, key auto-suggest, color, visibility). FR-PROJ-001/002. Template field dropped — `CreateProject` seeds fixed `DefaultColumns`, no template param (ADR-017). 402 → plan-limit + Billing CTA, 409 → key taken, 422 → field errors.
- [x] 7.5.3 `/projects/{projectKey}` Kanban board — dnd-kit columns + cards, WIP indicators (amber at limit). FR-PROJ-004/005. Key→UUID resolved client-side (no by-key route, ADR-017); inline add-task; add-column for ADMIN+.
- [x] 7.5.4 Drag-drop → LexoRank move with optimistic `setQueryData` + rollback + toast on 409. FR-PROJ-005. Client rank via `lib/board/rank.ts` (faithful port of `pkg/rank`, torture-tested).
- [x] 7.5.5 Board filters (text/priority/assignee — label filter impossible, board DTO carries no labels, ADR-017) + bulk multi-select (move/assign/unassign/add-label via `/tasks/bulk`). FR-TASK-008.
- [x] 7.5.6 `/projects/{projectKey}/list` sortable table view + `/settings` (general → `updateProject`; columns add/rename/WIP/reorder via `rank.Between`/delete with `?target=`; archive/unarchive). FR-PROJ-003/004/006. ADMIN+ gated; project-member management deferred (needs org member list — §7).

## Section 6 — Task detail + search/trash/my-tasks  ✅ (commit below)

- [x] 7.6.1 `/projects/{projectKey}/tasks/{taskNumber}` — full page. 02 §4. Number→id resolved via the board projection (no by-number route / no `number` search filter — ADR-018); modal-over-board interception deferred to full page (ADR-018).
- [x] 7.6.2 Inline edit of every field (title/description/assignee/priority/due/labels; UpdateTask replaces the whole set) + activity timeline. FR-TASK-001/002. Assignee/actor names hydrated from `GET /orgs/{id}/members` (`use-members.ts`) since task DTOs carry only ids.
- [x] 7.6.3 Subtasks checklist + comments (15-min edit marker, author/LEAD delete, @mention autocomplete). FR-TASK-003/005. @mention lists **org** members (project-member subset deferred, ADR-018); bodies render pre-wrapped (no markdown renderer this section).
- [x] 7.6.4 Attachments UI — request-upload → presigned PUT direct to MinIO (no auth header) → confirm; download via presigned GET. FR-TASK-006. 402 → storage-limit toast.
- [x] 7.6.5 `/search?q=` (URL-bound q + project/assignee/priority/label facets, Suspense-wrapped) + `/my-tasks` (assignee=self, grouped by project) + `/trash` (org-level = per-project ListTrash fan-out; restore-only, no hard-purge endpoint — ADR-018). FR-TASK-007/009. Sidebar nav added for all three.

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
