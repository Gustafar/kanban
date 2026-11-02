package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"kanban_board/internal/proc"
	"kanban_board/internal/store"
)

const polishTmpl = `Você está ajudando a refinar uma tarefa de um quadro kanban ANTES dela ser executada por uma IA. Converse com o usuário: tire dúvidas, aponte riscos ou ambiguidades, e sugira como deixar o título e a descrição mais claros e completos. Você pode explorar o código do projeto para entender o contexto, mas não deve editar nada, rodar comandos que alterem algo nem fazer commits — esta é só uma conversa de planejamento.

# Tarefa
Título: %s
Descrição: %s
%s
Responda em texto corrido, em português. Se fizer sentido, termine propondo uma nova versão do título e/ou da descrição, claramente identificada.`

// Polish starts a read-only conversation with the AI to discuss and refine a card before it runs:
// no file edits, no worktree, no commits — just discussion, posted back as an AI comment.
func (r *Runner) Polish(card store.Card) {
	if r.store.IsPolishing(card.ID) {
		return
	}
	r.store.SetPolishing(card.ID, true)
	r.publishCard(card.ID)
	go func() {
		defer func() {
			r.store.SetPolishing(card.ID, false)
			r.publishCard(card.ID)
		}()
		r.runPolish(card)
	}()
}

func (r *Runner) runPolish(card store.Card) {
	project, err := r.store.GetProject(card.ProjectID)
	if err != nil {
		return
	}
	history, _ := r.store.ListComments(card.ID)
	var convo strings.Builder
	for _, c := range history {
		who := "Usuário"
		if c.Author == "ai" {
			who = "IA"
		}
		fmt.Fprintf(&convo, "\n%s: %s\n", who, c.Body)
	}
	extra := ""
	if convo.Len() > 0 {
		extra = "\n# Conversa até agora\n" + convo.String()
	}
	prompt := fmt.Sprintf(polishTmpl, card.Title, card.Description, extra) + r.attachmentBlock(card.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	args := append([]string{"-p", "--output-format", "json", "--permission-mode", "plan", "--allowedTools", "Read,Glob,Grep"}, modelArgs(card)...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	proc.Hide(cmd)
	cmd.Dir = project.Path
	cmd.Stdin = strings.NewReader(prompt)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		r.addPolishComment(card.ID, "Não consegui pensar sobre esta tarefa agora: "+msg)
		return
	}
	text, err := parsePolishResult(out.Bytes())
	if err != nil || text == "" {
		return
	}
	r.addPolishComment(card.ID, text)
}

func (r *Runner) addPolishComment(cardID int64, body string) {
	if c, err := r.store.AddComment(cardID, "ai", body); err == nil {
		r.publish("comment", c)
	}
}

// parsePolishResult reads the CLI's JSON envelope and pulls out the plain-text reply.
func parsePolishResult(raw []byte) (string, error) {
	var env struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("saída inesperada do claude: %w", err)
	}
	if env.IsError {
		return "", fmt.Errorf("claude respondeu com erro: %s", env.Result)
	}
	return strings.TrimSpace(env.Result), nil
}
