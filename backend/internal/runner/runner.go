package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"kanban_board/internal/notify"
	"kanban_board/internal/store"
)

type Publisher func(typ string, data any)

type Runner struct {
	store   *store.Store
	dataDir string
	publish Publisher

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc // runID -> cancel
	busy    map[int64]bool               // projectID -> a direct-mode run is editing the folder
}

func New(s *store.Store, dataDir string, p Publisher) *Runner {
	return &Runner{store: s, dataDir: dataDir, publish: p, cancels: map[int64]context.CancelFunc{}, busy: map[int64]bool{}}
}

func (r *Runner) lockProject(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy[id] {
		return false
	}
	r.busy[id] = true
	return true
}

func (r *Runner) unlockProject(id int64) {
	r.mu.Lock()
	delete(r.busy, id)
	r.mu.Unlock()
}

const promptTmpl = `Você está trabalhando em uma tarefa de um quadro kanban, dentro de um git worktree isolado (branch própria).

# Tarefa: %s

%s

Regras:
- Altere apenas o necessário para cumprir a tarefa neste diretório.
- Rode os testes/build do projeto quando existirem.
- Ao final, faça commit das mudanças com uma mensagem clara e resuma o que foi feito.`

const promptDirectTmpl = `Você está trabalhando em uma tarefa de um quadro kanban, diretamente na pasta do projeto (que não usa git).

# Tarefa: %s

%s

Regras:
- Altere apenas o necessário para cumprir a tarefa.
- Rode os testes/build do projeto quando existirem.
- Não inicialize git nem faça commits. Ao final, resuma o que foi feito.`

// Request is called when a card enters "doing" (or the user hits "run").
// In "approval" mode it parks the run until Approve; in "auto" mode it starts right away.
func (r *Runner) Request(card store.Card, message string) (store.Run, error) {
	if run, err := r.store.LatestRun(card.ID); err == nil && (run.Status == "running" || run.Status == "awaiting_approval") {
		return run, nil // already queued/active
	}
	tmpl := promptTmpl
	if p, err := r.store.GetProject(card.ProjectID); err == nil && p.Isolation == "direct" {
		tmpl = promptDirectTmpl
	}
	prompt := fmt.Sprintf(tmpl, card.Title, card.Description)
	if message != "" {
		prompt = message // follow-up on an existing session
	}
	status := "awaiting_approval"
	if card.Mode == "auto" {
		status = "running"
	}
	run, err := r.store.CreateRun(card.ID, status, prompt)
	if err != nil {
		return run, err
	}
	r.publish("run", run)
	if status == "running" {
		go r.execute(run.ID)
	} else {
		go notify.Toast("Aprovação necessária", card.Title)
		r.publish("notify", map[string]any{"kind": "approval", "title": "Aprovação necessária", "body": card.Title, "cardId": card.ID})
	}
	return run, nil
}

func (r *Runner) Approve(runID int64) error {
	run, err := r.store.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status != "awaiting_approval" {
		return fmt.Errorf("execução não está aguardando aprovação (status: %s)", run.Status)
	}
	if err := r.store.UpdateRun(runID, "running", "", "", "", "", nil, nil); err != nil {
		return err
	}
	run.Status = "running"
	r.publish("run", run)
	go r.execute(runID)
	return nil
}

func (r *Runner) Cancel(runID int64) error {
	r.mu.Lock()
	cancel, ok := r.cancels[runID]
	r.mu.Unlock()
	if ok {
		cancel()
		return nil
	}
	run, err := r.store.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status == "awaiting_approval" {
		now := time.Now()
		_ = r.store.UpdateRun(runID, "cancelled", "", "", "", "", nil, &now)
		run, _ = r.store.GetRun(runID)
		r.publish("run", run)
	}
	return nil
}

func (r *Runner) emit(runID int64, kind, data string) {
	if ev, err := r.store.AddEvent(runID, kind, data); err == nil {
		r.publish("run_event", ev)
	}
}

func (r *Runner) execute(runID int64) {
	run, err := r.store.GetRun(runID)
	if err != nil {
		return
	}
	card, _ := r.store.GetCard(run.CardID)
	project, err := r.store.GetProject(card.ProjectID)
	if err != nil {
		r.finish(run, card, "failed", "projeto não encontrado")
		return
	}

	direct := project.Isolation == "direct"
	var shadow *Shadow
	var branch, wt string
	if direct {
		// one editor at a time: the folder is shared with you and with other cards
		if !r.lockProject(project.ID) {
			r.finish(run, card, "failed", "outra execução já está editando a pasta deste projeto; aguarde ela terminar")
			return
		}
		defer r.unlockProject(project.ID)
		wt = project.Path
		if shadow, err = NewShadow(r.dataDir, project.ID, project.Path); err == nil {
			var base string
			if base, err = shadow.Snapshot("antes: " + card.Title); err == nil {
				err = r.store.SetRunBaseRef(runID, base)
			}
		}
		if err != nil {
			r.finish(run, card, "failed", "não consegui salvar o estado da pasta: "+err.Error())
			return
		}
		now := time.Now()
		_ = r.store.UpdateRun(runID, "running", "", "", "", "", &now, nil)
		r.emit(runID, "system", "executando direto em "+wt+" (snapshot salvo para diff e desfazer)")
	} else {
		if branch, wt, err = ensureWorktree(project.Path, project.Name, card.ID, card.Title); err != nil {
			r.finish(run, card, "failed", err.Error())
			return
		}
		now := time.Now()
		_ = r.store.UpdateRun(runID, "running", "", branch, wt, "", &now, nil)
		r.emit(runID, "system", "worktree: "+wt+" ("+branch+")")
	}

	// resume the previous Claude session for follow-ups on the same card
	resume := ""
	if runs, _ := r.store.ListRuns(card.ID); len(runs) > 0 {
		for _, pr := range runs {
			if pr.ID != runID && pr.SessionID != "" {
				resume = pr.SessionID
				break
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancels[runID] = cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		delete(r.cancels, runID)
		r.mu.Unlock()
	}()

	allowed := os.Getenv("KB_ALLOWED_TOOLS")
	if allowed == "" {
		allowed = "Bash,Edit,Write,Read,Glob,Grep"
	}
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--permission-mode", "acceptEdits", "--allowedTools", allowed}
	if resume != "" {
		args = append(args, "--resume", resume)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = wt
	cmd.Stdin = strings.NewReader(run.Prompt) // stdin avoids Windows quoting issues
	stdout, _ := cmd.StdoutPipe()
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		r.finish(run, card, "failed", "não consegui iniciar o claude: "+err.Error())
		return
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	resultErr := ""
	isError := false
	for sc.Scan() {
		if e, ok := r.handleLine(runID, sc.Bytes()); ok {
			isError, resultErr = e.isError, e.err
		}
	}
	waitErr := cmd.Wait()

	switch {
	case ctx.Err() != nil:
		r.finish(run, card, "cancelled", "")
	case isError || waitErr != nil:
		msg := resultErr
		if msg == "" {
			msg = strings.TrimSpace(stderr.String())
		}
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		r.finish(run, card, "failed", msg)
	default:
		if direct {
			if _, err := shadow.Snapshot("depois: " + card.Title); err != nil {
				r.emit(runID, "error", "não consegui salvar o snapshot final: "+err.Error())
			}
		} else {
			_ = commitAll(wt, "kanban: "+card.Title)
		}
		r.finish(run, card, "succeeded", "")
	}
}

type resultInfo struct {
	isError bool
	err     string
}

// handleLine translates one stream-json line into stored/broadcast events.
func (r *Runner) handleLine(runID int64, line []byte) (resultInfo, bool) {
	var m struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
		IsError   bool   `json:"is_error"`
		Result    string `json:"result"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &m) != nil {
		return resultInfo{}, false
	}
	if m.SessionID != "" {
		_ = r.store.UpdateRun(runID, "", m.SessionID, "", "", "", nil, nil)
	}
	switch m.Type {
	case "assistant":
		var blocks []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(m.Message.Content, &blocks) != nil {
			return resultInfo{}, false
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					r.emit(runID, "text", t)
				}
			case "tool_use":
				r.emit(runID, "tool", b.Name+" "+truncate(string(b.Input), 300))
			}
		}
	case "result":
		kind := "result"
		if m.IsError {
			kind = "error"
		}
		r.emit(runID, kind, m.Result)
		return resultInfo{isError: m.IsError, err: map[bool]string{true: m.Result}[m.IsError]}, true
	}
	return resultInfo{}, false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (r *Runner) finish(run store.Run, card store.Card, status, errMsg string) {
	now := time.Now()
	_ = r.store.UpdateRun(run.ID, status, "", "", "", errMsg, nil, &now)
	if errMsg != "" {
		r.emit(run.ID, "error", errMsg)
	}
	updated, _ := r.store.GetRun(run.ID)
	r.publish("run", updated)

	title, body := "", card.Title
	switch status {
	case "succeeded":
		title = "Pronto para revisão"
		if c, err := r.store.GetCard(card.ID); err == nil && c.Column == "doing" {
			if mv, err := r.store.MoveCard(card.ID, "review", c.Position); err == nil {
				r.publish("card", mv)
			}
		}
	case "failed":
		title = "Execução falhou"
	}
	if c, err := r.store.GetCard(card.ID); err == nil {
		r.publish("card", c)
	}
	if title != "" {
		go notify.Toast(title, body)
		r.publish("notify", map[string]any{"kind": status, "title": title, "body": body, "cardId": card.ID})
	}
}
