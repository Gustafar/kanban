# Kanban + Claude

Kanban pessoal e local: cada card pode ser executado pelo Claude Code num git worktree isolado. Visão geral, fluxo, CLI, MCP e variáveis de ambiente estão no [README.md](README.md); aqui ficam só o que você precisa para mexer no código.

## Estrutura

- `backend/` — Go (módulo `kanban_board`), SQLite via `modernc.org/sqlite`, sem CGO.
  - `cmd/server` (servidor, compilado como app sem janela), `cmd/kb` (CLI e servidor MCP; também vira `cardume.exe`).
  - `internal/api` rotas HTTP + SSE · `internal/runner` execução do `claude -p`, worktrees, fila · `internal/store` SQLite · `internal/selfupdate` reinstalação do próprio app · `internal/titler` título/tags por IA · `internal/limits` uso do plano · `internal/notify` avisos do Windows · `internal/proc` utilitários de processo.
  - `web/dist` é o frontend compilado, embutido no servidor (gerado, não edite).
- `frontend/` — React 18 + Vite + TypeScript. `src/App.tsx` (shell, barra lateral, atalhos), `src/useBoard.ts` (estado e SSE), `src/api.ts` (chamadas), `src/components/`.

## Comandos

- Frontend: `cd frontend && npm run build` (roda `tsc --noEmit` e o Vite, escrevendo em `backend/web/dist`). Dev com hot reload: `npm run dev`.
- Backend: `cd backend && go build ./... && go vet ./...`; testes: `go test ./...`.
- Instalar o app rodando (compila tudo, troca os executáveis, reinicia): `.\install.ps1`. Windows + PowerShell; o usuário roda isso, não rode sem pedir (para o servidor em uso).

## Convenções

- Interface e mensagens ao usuário em **português (pt-BR)**; código e comentários em inglês, comentários curtos e só onde o motivo não é óbvio.
- Estilos em CSS puro: `styles.css` (base e layout), `detail.css` (gaveta do card), `dialogs.css`, `popover.css`. Use as variáveis (`--ink`, `--line`, `--lane`, …) existentes.
- Sem dependências novas sem necessidade; o frontend usa só React e dnd-kit.
- Mobile: o layout muda em `@media (max-width: 900px)` (barra lateral vira faixa horizontal); confira os dois ao mexer no layout.

## Cuidados

- O servidor em execução é a cópia instalada em `backend/bin`; mudar o código não muda o app até reinstalar.
- A atualização pendente do app (`selfupdate`) e a fila de execuções vivem só na memória do servidor.
- Execuções em projetos sem git editam a pasta direto (com snapshot); só uma por pasta por vez.
- `move_card` e `create_card` (MCP) recusam a coluna `doing` de propósito: só o usuário inicia execuções.
