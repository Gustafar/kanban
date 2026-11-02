# Kanban + Claude

Personal, local kanban board: each card can be run by Claude Code in an isolated git worktree. Overview, flow, CLI, MCP and environment variables are in [README.md](README.md); this file only holds what you need to change the code.

## Layout

- `backend/` — Go (module `kanban_board`), SQLite via `modernc.org/sqlite`, no CGO.
  - `cmd/server` (the server, built as a windowless app), `cmd/kb` (CLI and MCP server; also built as `cardume.exe`).
  - `internal/api` HTTP routes + SSE · `internal/runner` `claude -p` execution, worktrees, queue · `internal/store` SQLite · `internal/selfupdate` reinstalling the app itself · `internal/titler` AI title/tags · `internal/limits` plan usage · `internal/proc` process helpers.
  - `web/dist` is the compiled frontend, embedded in the server (generated, do not edit).
- `frontend/` — React 18 + Vite + TypeScript. `src/App.tsx` (shell, sidebar, shortcuts), `src/useBoard.ts` (state and SSE), `src/api.ts` (calls), `src/components/`.

## Commands

- Frontend: `cd frontend && npm run build` (runs `tsc --noEmit` and Vite, writing to `backend/web/dist`). Hot reload: `npm run dev`.
- Backend: `cd backend && go build ./... && go vet ./...`; tests: `go test ./...`.
- Install the running app (builds everything, swaps the executables, restarts): `.\install.ps1`. Windows + PowerShell; the user runs this, do not run it unasked (it stops the server in use).

## Conventions

- Code, comments, commit messages and docs in **English**; the app **interface and user-facing messages stay in Portuguese (pt-BR)**. Keep comments short and only where the reason is not obvious.
- Plain CSS: `styles.css` (base and layout), `detail.css` (card drawer), `dialogs.css`, `popover.css`. Use the existing variables (`--ink`, `--line`, `--lane`, …).
- No new dependencies unless needed; the frontend only uses React and dnd-kit.
- Mobile: the layout changes at `@media (max-width: 900px)` (sidebar becomes a horizontal strip); check both when touching layout.

## Gotchas

- The running server is the copy installed in `backend/bin`; changing the code does not change the app until you reinstall.
- The pending app update (`selfupdate`) is persisted in the `settings` table (survives a restart); the run queue still lives only in the server's memory.
- Runs in projects without git edit the folder directly (with a snapshot); only one per folder at a time.
- `move_card` and `create_card` (MCP) refuse the `doing` column on purpose: only the user starts runs.
- `PLANO.md` (git-ignored) is the shared work plan between sessions; read it before starting and update it when you finish an item.
