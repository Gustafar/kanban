package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kanban_board/internal/limits"
	"kanban_board/internal/proc"
	"kanban_board/internal/store"
)

var ErrChatBusy = errors.New("a IA ainda está respondendo nesta conversa")

const chatSystemPrompt = `Você é o assistente do quadro kanban do projeto "%s" (pasta: %s). Converse com o usuário em português e ajude a organizar o trabalho: criar cards a partir do que ele descrever, mover cards entre colunas, ajustar títulos e descrições e registrar notas.

Como agir:
- Use as ferramentas do quadro (list_cards, get_card, create_card, update_card, move_card, add_note). Elas só enxergam este projeto. Consulte os cards existentes antes de criar, para não duplicar.
- Colunas: backlog, todo, review, done. Você NÃO pode mover nem criar cards em "doing": só o usuário inicia execuções.
- Ao criar um card, escreva um título curto e uma descrição que outra execução da IA consiga cumprir sozinha (contexto, o que fazer, critério de pronto). Se o pedido do usuário tiver várias tarefas, crie um card por tarefa.
- Pode ler o código do projeto (Read, Glob, Grep) para entender o contexto, mas não altere arquivos nem rode comandos.
- Ao terminar, diga em poucas linhas o que foi feito (ex.: "Criei 3 cards no backlog: #12, #13, #14"). Se o pedido for ambíguo, pergunte antes de agir.`

// chatMCPConfig writes an MCP config whose board tools are locked to one project (KB_PROJECT_ID).
func (r *Runner) chatMCPConfig(chatID, projectID int64) string {
	if r.mcpBin == "" {
		return ""
	}
	cfg := map[string]any{"mcpServers": map[string]any{"kanban": map[string]any{
		"command": r.mcpBin,
		"args":    []string{"mcp"},
		"env":     map[string]string{"KB_URL": r.baseURL, "KB_TOKEN": r.token, "KB_PROJECT_ID": fmt.Sprint(projectID)},
	}}}
	dir := filepath.Join(r.dataDir, "mcp")
	if os.MkdirAll(dir, 0o700) != nil {
		return ""
	}
	path := filepath.Join(dir, fmt.Sprintf("chat-%d.json", chatID))
	b, _ := json.Marshal(cfg)
	if os.WriteFile(path, b, 0o600) != nil { // holds the token: owner-only
		return ""
	}
	return path
}

// ChatBusy reports whether the AI is answering in this chat right now.
func (r *Runner) ChatBusy(chatID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.chatCancels[chatID]
	return ok
}

// CancelChat interrupts the answer in progress, if any.
func (r *Runner) CancelChat(chatID int64) {
	r.mu.Lock()
	cancel := r.chatCancels[chatID]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Runner) publishChat(chatID int64) {
	if c, err := r.store.GetChat(chatID); err == nil {
		c.Busy = r.ChatBusy(chatID)
		r.publish("chat", c)
	}
}

func (r *Runner) chatMessage(chatID int64, role, body string) {
	if m, err := r.store.AddChatMessage(chatID, role, body); err == nil {
		r.publish("chat_message", m)
	}
}

func (r *Runner) chatAttachDir(chatID int64) string {
	return filepath.Join(r.dataDir, "attachments", fmt.Sprintf("chat-%d", chatID))
}

// SendChat stores the user's message (with the files already uploaded to this chat) and starts the
// AI's answer in the background.
func (r *Runner) SendChat(chat store.Chat, text string, attachIDs []int64) error {
	var files []store.ChatAttachment
	for _, id := range attachIDs {
		a, err := r.store.GetChatAttachment(id)
		if err != nil || a.ChatID != chat.ID || a.MessageID != 0 {
			return errors.New("anexo inválido ou já enviado")
		}
		files = append(files, a)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if _, busy := r.chatCancels[chat.ID]; busy {
		r.mu.Unlock()
		cancel()
		return ErrChatBusy
	}
	r.chatCancels[chat.ID] = cancel
	r.mu.Unlock()

	if m, err := r.store.AddChatMessage(chat.ID, "user", text); err == nil {
		if r.store.LinkChatAttachments(m.ID, attachIDs) == nil {
			for i := range files {
				files[i].MessageID = m.ID
			}
			m.Attachments = append(m.Attachments, files...)
		}
		r.publish("chat_message", m)
	}
	r.publishChat(chat.ID)

	prompt := text
	if len(files) > 0 {
		if prompt == "" {
			prompt = "Veja os arquivos anexados."
		}
		var b strings.Builder
		b.WriteString(prompt + "\n\nArquivos anexados a esta mensagem (locais; use a ferramenta Read para ver imagens e PDFs):\n")
		for _, a := range files {
			fmt.Fprintf(&b, "- %s (%s)\n", a.Path, a.Name)
		}
		prompt = b.String()
	}
	go func() {
		defer func() {
			cancel()
			r.mu.Lock()
			delete(r.chatCancels, chat.ID)
			r.mu.Unlock()
			r.publishChat(chat.ID)
		}()
		r.runChat(ctx, chat, prompt)
	}()
	return nil
}

func (r *Runner) runChat(ctx context.Context, chat store.Chat, text string) {
	project, err := r.store.GetProject(chat.ProjectID)
	if err != nil {
		r.chatMessage(chat.ID, "error", "projeto não encontrado")
		return
	}
	mcpCfg := r.chatMCPConfig(chat.ID, project.ID)
	if mcpCfg == "" {
		r.chatMessage(chat.ID, "error", "as ferramentas do quadro não estão disponíveis (servidor sem MCP configurado)")
		return
	}
	// default permission mode: anything not listed here is denied, so no edits and no shell
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--allowedTools", "Read,Glob,Grep,mcp__kanban",
		"--append-system-prompt", fmt.Sprintf(chatSystemPrompt, project.Name, project.Path),
		"--mcp-config", mcpCfg}
	args = append(args, modelArgs(store.Card{Model: chat.Model, Effort: chat.Effort})...)
	if _, err := os.Stat(r.chatAttachDir(chat.ID)); err == nil {
		args = append(args, "--add-dir", r.chatAttachDir(chat.ID)) // lets the Read tool open the files
	}
	if chat.SessionID != "" {
		args = append(args, "--resume", chat.SessionID)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	proc.Hide(cmd)
	cmd.Dir = project.Path
	cmd.Stdin = strings.NewReader(text) // stdin avoids Windows quoting issues
	stdout, _ := cmd.StdoutPipe()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		r.chatMessage(chat.ID, "error", "não consegui iniciar o claude: "+err.Error())
		return
	}

	session, resultErr := "", ""
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if s, e := r.handleChatLine(chat.ID, sc.Bytes()); s != "" || e != "" {
			if s != "" {
				session = s
			}
			if e != "" {
				resultErr = e
			}
		}
	}
	waitErr := cmd.Wait()
	if session != "" && session != chat.SessionID {
		_ = r.store.SetChatSession(chat.ID, session)
	}
	switch {
	case ctx.Err() != nil:
		r.chatMessage(chat.ID, "error", "Resposta interrompida.")
	case resultErr != "":
		r.chatMessage(chat.ID, "error", resultErr)
	case waitErr != nil:
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		r.chatMessage(chat.ID, "error", msg)
	}
}

// handleChatLine stores the assistant's text and tool calls as messages. It returns the CLI session id
// when the line carries one, and an error text when the final result is an error.
func (r *Runner) handleChatLine(chatID int64, line []byte) (session, errText string) {
	var m struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		IsError   bool   `json:"is_error"`
		Result    string `json:"result"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &m) != nil {
		return "", ""
	}
	switch m.Type {
	case "rate_limit_event":
		if s, ok := limits.Parse(line); ok && r.limitsSink != nil {
			r.limitsSink(s)
		}
	case "assistant":
		var blocks []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(m.Message.Content, &blocks) != nil {
			return m.SessionID, ""
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					r.chatMessage(chatID, "assistant", t)
				}
			case "tool_use":
				name := strings.TrimPrefix(b.Name, "mcp__kanban__")
				r.chatMessage(chatID, "tool", name+" "+truncate(string(b.Input), 300))
			}
		}
	case "result":
		if m.IsError {
			return m.SessionID, m.Result
		}
	}
	return m.SessionID, ""
}
