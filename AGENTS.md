# AGENTS.md — Working Agreement for AI Agents

Applies to every AI coding agent in this repo (Claude Code, Codex, Cursor, …).
Human contributors should follow it too.

## 1. Never work directly on `main`

`main` is protected by convention. Before your **first** file edit in a task:

```bash
git switch -c <type>/<short-description>     # e.g. feat/task-watchers
# or, for parallel / isolated work:
git worktree add ../fluxboard-<slug> -b <type>/<short-description>
```

`<type>` is one of `feat` `fix` `refactor` `test` `docs` `chore` (matches the
commit-type list below).

- If you notice you are on `main`, stop and create the branch now — do not add
  "move to a branch" to the end of the task.
- One task = one branch. Don't pile unrelated changes onto an existing branch.
- Rebase on `origin/main` before opening the PR; don't merge `main` back in.

## 2. Commits

- **Conventional commits:** `feat:`, `fix:`, `refactor:`, `test:`, `docs:`,
  `chore:` — imperative subject, ≤ 72 chars.
- **Single concern per commit.** Don't bundle an unrelated drive-by fix into a
  feature commit; make it its own commit (or its own branch).
- Run the gate before committing (see §4). Don't commit red.
- Commit trailers: keep whatever attribution your tool is configured to add;
  don't strip co-author lines.

## 3. Pull requests

- Open a PR against `main`; do not push to `main` directly.
- PR description: what changed, why, how it was verified, and the `FR-XXX-NNN`
  / `ADR-NNN` IDs it advances (see `docs/`).
- Keep PRs reviewable — split large work into stacked branches.

## 4. Verification gate

Backend (`backend/` is the Go module root):

```bash
cd backend && go build ./... && go vet ./... && go test ./...
```

Frontend (`web/`):

```bash
cd web && pnpm install && pnpm typecheck && pnpm lint && pnpm build
```

Run the slice relevant to your change at minimum; run both if you touched the
API contract. CI (`.github/workflows/ci.yml`) enforces the same.

## 5. Project rules that are not negotiable

Full detail in [`docs/CLAUDE.md`](docs/CLAUDE.md) and the numbered specs. The
load-bearing ones:

- **Tenant isolation:** every tenant-scoped query goes through the
  tenant-scoped connection wrapper (RLS). Application layer filters too —
  defense in depth.
- **Money:** integer minor units, never float. **Timestamps:** `timestamptz`,
  UTC in the DB.
- **sqlc** is the source of truth for queries; never hand-edit generated code
  in `backend/internal/infrastructure/postgres/gen/`.
- **Tokens** (web): access token in memory, refresh token in an httpOnly
  cookie — never `localStorage`.
- **Migrations** split DDL from RLS policy. Check `docs/build/README.md` for the
  next free migration number.

## 6. When reality conflicts with the docs

Stop and surface it. Propose the change as an ADR entry in
`docs/03-ARCHITECTURE.md` §ADR before implementing — don't silently improvise.
