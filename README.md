# Kanban + Claude

Kanban pessoal e local. Cada projeto aponta para um repositório git; ao mover um card para **Em execução**, o Claude Code (seu login, via CLI `claude`) trabalha num git worktree isolado e o card vai para **Revisão** quando termina.

## Rodar

```powershell
.\start.ps1
```

Abra a URL `local:` ou `rede:` impressa no terminal (traz `?token=...`, que vira cookie). O token fica em `%APPDATA%\kanban-board\token`.

## Dois modos por projeto

- **Cópia isolada (git)**: a pasta precisa ser um repositório git local com um commit (sem GitHub). A IA trabalha num worktree em outra branch; você revisa o diff e faz merge.
- **Direto na pasta**: serve para pastas sem git. A IA edita a pasta; antes de cada execução o app salva um snapshot num git "sombra" em `%APPDATA%\kanban-board\shadow\` (o projeto não ganha `.git`). Dá para ver o diff e "Desfazer na pasta". Só uma execução por vez edita a mesma pasta.

Ao criar o projeto o app detecta o modo pela pasta, e o diálogo oferece "Iniciar git nesta pasta".

## Fluxo

- Card **Com aprovação**: ao entrar em execução fica "Aguardando sua aprovação" até você aprovar. **Automático**: roda sozinho.
- Cada execução cria a branch `kb/card-<id>-<slug>` em `<pasta-do-projeto>\..\.kanban-worktrees\`. Na revisão você vê o diff, faz merge ou descarta.
- "Pedir ajuste" continua a mesma sessão do Claude (`--resume`).
- Avisos: toast do Windows (servidor) e notificação do navegador (só em localhost/HTTPS).

## CLI

`backend\bin\kb.exe` (ver `kb` sem argumentos): `projects`, `project-add`, `ls`, `add`, `move`, `run`, `approve`, `cancel`, `logs`, `diff`, `merge`.

## MCP (a IA usa o board)

`kb mcp` é um servidor MCP (stdio) com as ferramentas `list_projects`, `list_cards`, `get_card`, `create_card`, `update_card`, `add_note` e `move_card`. `move_card` e `create_card` recusam a coluna `doing`: só você inicia execuções.

- **Nas execuções do board**: ligado automaticamente (o servidor precisa achar o `kb.exe` ao lado do `server.exe`, ou use `KB_BIN`). Cada execução recebe o próprio card como padrão, então `add_note` e `get_card` funcionam sem informar o id.
- **No seu Claude Code**, para conversar com o board de qualquer pasta (o `kb` lê o token sozinho em `%APPDATA%\kanban-board\token`):

```powershell
claude mcp add kanban --scope user -- C:\Projetos\kanban_board\backend\bin\kb.exe mcp
```

## Config (env)

| Var | Padrão |
|---|---|
| `KB_ADDR` | `:7777` |
| `KB_DATA` | `%APPDATA%\kanban-board` |
| `KB_TOKEN` | gerado e salvo no 1º start |
| `KB_ALLOWED_TOOLS` | `Bash,Edit,Write,Read,Glob,Grep` (ferramentas liberadas para o Claude nos worktrees) |

## Desenvolvimento

Backend: `cd backend && go run ./cmd/server`. Frontend com hot reload: `cd frontend && npm run dev` (proxy de `/api` para `:7777`; abra o backend uma vez com `?token=` para ganhar o cookie).
