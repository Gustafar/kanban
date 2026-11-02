# Kanban + Claude

Kanban pessoal e local. Cada projeto aponta para um repositório git; ao mover um card para **Em execução**, o Claude Code (seu login, via CLI `claude`) trabalha num git worktree isolado e o card vai para **Revisão** quando termina.

## Instalar (recomendado)

```powershell
.\install.ps1     # compila, sobe com o Windows (sem janela), cria o atalho "Kanban"
.\uninstall.ps1   # remove a inicialização e os atalhos (os dados ficam)
```

Depois disso nada precisa ficar aberto: o servidor roda em segundo plano, o log fica em `%APPDATA%\kanban-board\server.log` e `cardume` (em qualquer pasta, num terminal novo) abre o board já no projeto daquela pasta, criando o projeto se ele ainda não existir; `cardume <caminho>` faz o mesmo para outra pasta. Depois de mudar o código, rode `.\install.ps1` de novo.

## Rodar só uma vez (com terminal)

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

## Limite de execuções em paralelo

No máximo N execuções da IA rodam ao mesmo tempo (padrão 2); as demais ficam **na fila** e começam na ordem de chegada quando uma vaga abre. Dá para tirar um card da fila na gaveta. O limite (1 a 8) é ajustado com o − / + no rodapé da barra lateral e fica salvo; `KB_MAX_PARALLEL` define o valor inicial. Em projetos "direto na pasta" (sem git) só uma execução edita a pasta por vez, para não se atrapalharem: as outras esperam na fila (sem ocupar uma vaga) e começam sozinhas. Projetos com git isolam cada execução num worktree e rodam em paralelo de verdade. A fila vive na memória: se o servidor reiniciar, execuções em andamento ou na fila são marcadas como falhas.

## Barra de captura, tags e anexos

- A barra no centro inferior cria o card a partir de texto livre: o card aparece na hora com um título provisório e, em alguns segundos, a IA (Haiku, via seu `claude`) troca pelo título final e adiciona tags. O texto que você escreveu vira a descrição, que é o pedido para a IA executar. `Enter` cria, `Shift+Enter` quebra a linha, `n` foca a barra.
- Cole um print (Ctrl+V), arraste arquivos ou use o clipe, na barra ou na gaveta do card. Limite de 15 MB por arquivo. Ficam em `%APPDATA%\kanban-board\attachments\`.
- A IA recebe o caminho dos anexos no pedido e lê imagens e PDFs com a ferramenta Read (o MCP `get_card` também lista os anexos).
- Tags: editáveis na gaveta, e a busca encontra por tag (`#bug`).

## Comentários, busca e atalhos

- Cada card tem comentários (aba na gaveta). "Pedir ajuste" vira um comentário seu, o resumo final da IA vira um comentário dela, e a IA também escreve e lê comentários pelo MCP (`add_note`, `get_card`).
- Busca por título e descrição, e filtros "Precisa de mim" (aguardando aprovação, com falha ou em revisão) e "IA trabalhando".
- Atalhos: `n` novo card, `/` buscar, `1`–`9` trocar de projeto, `Esc` fechar, `?` ajuda.

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
