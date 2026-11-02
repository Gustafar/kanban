# Kanban + Claude

A personal, local kanban board where Claude Code does the work. You write a task, move the card to **In progress**, and the AI works in an isolated git worktree without touching your branch. When it finishes, the card moves to **Review**: you see the diff, ask for tweaks in the same session, then merge or discard.

**Highlights**

- Isolated runs per card (git worktree), or directly in the folder with a snapshot and "undo" for projects without git.
- Queue with a parallel-run limit; manual approval or automatic start per card.
- Capture bar: free text becomes a card, and the AI writes the title and tags.
- Model and effort per card, with a plan-usage meter.
- MCP server and CLI (`cardume`): the AI and you can use the board from any folder.
- Runs in the background on Windows, uses your Claude login (no API key), and can update itself from a card.

**Stack:** Go (SQLite, no CGO) backend; React, TypeScript and Vite frontend.

> The app interface is in Portuguese (pt-BR). Labels quoted below are shown in English for readability.

Each project points to a local git repository; when you move a card to **In progress**, Claude Code (your login, via the `claude` CLI) works in an isolated git worktree and the card goes to **Review** when it is done.

## Install (recommended)

```powershell
.\install.ps1     # builds, starts with Windows (no window), creates the "Kanban" shortcut
.\uninstall.ps1   # removes the startup entry and shortcuts (your data stays)
```

After that nothing needs to stay open: the server runs in the background, the log is at `%APPDATA%\kanban-board\server.log`, and `cardume` (in any folder, in a new terminal) opens the board on that folder's project, creating the project if it does not exist yet; `cardume <path>` does the same for another folder. After changing the code, run `.\install.ps1` again.

## Run once (with a terminal)

```powershell
.\start.ps1
```

Open the `local:` or `rede:` URL printed in the terminal (it carries `?token=...`, which becomes a cookie). The token is stored in `%APPDATA%\kanban-board\token`.

## Two modes per project

- **Isolated copy (git)**: the folder must be a local git repository with at least one commit (no GitHub needed). The AI works in a worktree on another branch; you review the diff and merge.
- **Directly in the folder**: for folders without git. The AI edits the folder; before each run the app saves a snapshot in a "shadow" git at `%APPDATA%\kanban-board\shadow\` (the project does not get a `.git`). You can see the diff and "Undo in folder". Only one run at a time edits the same folder.

When you create a project the app detects the mode from the folder, and the dialog offers "Start git in this folder".

## Flow

- **With approval** card: when it enters execution it stays "Awaiting your approval" until you approve. **Automatic**: runs on its own.
- Each run creates the branch `kb/card-<id>-<slug>` in `<project-folder>\..\.kanban-worktrees\`. In review you see the diff and merge or discard.
- "Ask for changes" continues the same Claude session (`--resume`).
- Notifications: the bell in the top bar (in-app list) and a browser notification when the tab is in the background (localhost/HTTPS only).

## Parallel run limit

At most N AI runs go at the same time (default 2); the rest are **queued** and start in arrival order when a slot opens. You can take a card out of the queue in the drawer. The limit (1 to 8) is set with the − / + in the sidebar footer and is saved; `KB_MAX_PARALLEL` sets the initial value. In "directly in the folder" projects (no git) only one run edits the folder at a time, so they do not step on each other: the others wait in the queue (without using a slot) and start on their own. Git projects isolate each run in a worktree and run truly in parallel. The queue lives in memory: if the server restarts, runs in progress or queued are marked as failed.

## Capture bar, tags and attachments

- The bar at the bottom center creates a card from free text: the card appears instantly with a provisional title and, within a few seconds, the AI (Haiku, via your `claude`) replaces it with the final title and adds tags. The text you wrote becomes the description, which is the request sent to the AI. `Enter` creates, `Shift+Enter` breaks the line, `n` focuses the bar.
- Paste a screenshot (Ctrl+V), drag files, or use the paperclip, in the bar or in the card drawer. 15 MB limit per file. They are stored in `%APPDATA%\kanban-board\attachments\`.
- The AI receives the attachment paths in the request and reads images and PDFs with the Read tool (the MCP `get_card` also lists attachments).
- Tags: editable in the drawer, and search finds by tag (`#bug`).

## Comments, search and shortcuts

- Each card has comments (tab in the drawer). "Ask for changes" becomes a comment from you, the AI's final summary becomes a comment from it, and the AI also writes and reads comments through MCP (`add_note`, `get_card`).
- Search by title and description, and the filters "Needs me" (awaiting approval, failed or in review) and "AI working".
- Shortcuts: `n` new card, `/` search, `1`–`9` switch project, `Esc` close, `?` help.

## Collapsible sidebar

The button at the right of the sidebar header collapses it; when collapsed, the same icon appears at the top of the board to bring it back. The state is remembered in the browser.

## Updating the app from a card

When a card from this very repository enters review, the app offers "New updates". The button rebuilds the app from that card's worktree and restarts it, so you can try the change in the real app. The pending update is persisted (settings table), so it survives a server restart. Installer output goes to `%APPDATA%\kanban-board\install.log`; if the build fails before the restart, the server stays up and shows the error (a tail of that log) with a "Try again" button instead of leaving the UI waiting.

## CLI

`backend\bin\kb.exe` (run `kb` with no arguments for help): `projects`, `project-add`, `ls`, `add`, `move`, `run`, `approve`, `cancel`, `logs`, `diff`, `merge`.

## MCP (the AI uses the board)

`kb mcp` is an MCP server (stdio) with the tools `list_projects`, `list_cards`, `get_card`, `create_card`, `update_card`, `add_note` and `move_card`. `move_card` and `create_card` refuse the `doing` column: only you start runs.

- **In board runs**: enabled automatically (the server must find `kb.exe` next to `server.exe`, or use `KB_BIN`). Each run gets its own card as the default, so `add_note` and `get_card` work without an id.
- **In your own Claude Code**, to talk to the board from any folder (`kb` reads the token by itself from `%APPDATA%\kanban-board\token`):

```powershell
claude mcp add kanban --scope user -- C:\Projetos\kanban_board\backend\bin\kb.exe mcp
```

## Config (env)

| Var | Default |
|---|---|
| `KB_ADDR` | `:7777` |
| `KB_DATA` | `%APPDATA%\kanban-board` |
| `KB_TOKEN` | generated and saved on first start |
| `KB_ALLOWED_TOOLS` | `Bash,Edit,Write,Read,Glob,Grep` (tools allowed for Claude in worktrees) |
| `KB_BACKUP_INTERVAL` | `24h` (how often `kanban.db` is snapshotted into `backups/`; keeps the last 14) |

## Development

Backend: `cd backend && go run ./cmd/server`. Frontend with hot reload: `cd frontend && npm run dev` (proxies `/api` to `:7777`; open the backend once with `?token=` to get the cookie).
