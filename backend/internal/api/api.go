package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"kanban_board/internal/runner"
	"kanban_board/internal/store"
)

type Server struct {
	store   *store.Store
	hub     *Hub
	runner  *runner.Runner
	token   string
	dataDir string
	static  fs.FS // optional embedded frontend
}

func New(s *store.Store, dataDir, token string, static fs.FS) (*Server, *Hub) {
	hub := NewHub()
	srv := &Server{store: s, hub: hub, token: token, dataDir: dataDir, static: static}
	srv.runner = runner.New(s, dataDir, hub.Publish)
	// parallelism limit: saved setting, else KB_MAX_PARALLEL, else the default
	limit := runner.DefaultMaxParallel
	if n, err := strconv.Atoi(os.Getenv("KB_MAX_PARALLEL")); err == nil {
		limit = n
	}
	if v, ok := s.GetSetting("max_parallel"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	srv.runner.SetMax(limit)
	return srv, hub
}

// ConfigureMCP gives runs access to the board through `kb mcp` (bin = path to kb.exe).
func (s *Server) ConfigureMCP(bin, baseURL string) { s.runner.SetMCP(bin, baseURL, s.token) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", s.sse)

	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/inspect", s.inspect)
	mux.HandleFunc("POST /api/git/init", s.gitInit)
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PUT /api/projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)

	mux.HandleFunc("GET /api/projects/{id}/cards", s.listCards)
	mux.HandleFunc("POST /api/projects/{id}/cards", s.createCard)
	mux.HandleFunc("GET /api/cards/{id}", s.getCard)
	mux.HandleFunc("PUT /api/cards/{id}", s.updateCard)
	mux.HandleFunc("POST /api/cards/{id}/move", s.moveCard)
	mux.HandleFunc("DELETE /api/cards/{id}", s.deleteCard)
	mux.HandleFunc("POST /api/cards/{id}/run", s.runCard)
	mux.HandleFunc("GET /api/cards/{id}/runs", s.listRuns)
	mux.HandleFunc("POST /api/projects/{id}/quick", s.quickCard)
	mux.HandleFunc("GET /api/cards/{id}/attachments", s.listAttachments)
	mux.HandleFunc("POST /api/cards/{id}/attachments", s.addAttachment)
	mux.HandleFunc("GET /api/attachments/{id}", s.serveAttachment)
	mux.HandleFunc("DELETE /api/attachments/{id}", s.deleteAttachment)
	mux.HandleFunc("GET /api/cards/{id}/comments", s.listComments)
	mux.HandleFunc("POST /api/cards/{id}/comments", s.addComment)

	mux.HandleFunc("GET /api/runs/{id}/events", s.runEvents)
	mux.HandleFunc("POST /api/runs/{id}/approve", s.approveRun)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancelRun)
	mux.HandleFunc("GET /api/runs/{id}/diff", s.runDiff)
	mux.HandleFunc("POST /api/runs/{id}/merge", s.mergeRun)
	mux.HandleFunc("POST /api/runs/{id}/discard", s.discardRun)

	if s.static != nil {
		mux.Handle("/", spa(s.static))
	}
	return s.auth(mux)
}

// ---- auth: shared token, cookie-based so SSE and the PWA just work ----

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && s.tokenOK(t) {
			http.SetCookie(w, &http.Cookie{Name: "kb_token", Value: t, Path: "/", HttpOnly: true,
				SameSite: http.SameSiteStrictMode, MaxAge: 60 * 60 * 24 * 365})
			u := *r.URL
			q := u.Query()
			q.Del("token")
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
		ok := false
		if c, err := r.Cookie("kb_token"); err == nil && s.tokenOK(c.Value) {
			ok = true
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") && s.tokenOK(strings.TrimPrefix(h, "Bearer ")) {
			ok = true
		}
		if !ok {
			http.Error(w, "unauthorized: abra a URL com ?token=... impressa no terminal", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) {
		code = http.StatusNotFound
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

func validColumn(c string) bool {
	for _, v := range store.Columns {
		if v == c {
			return true
		}
	}
	return false
}

func validMode(m string) bool { return m == "approval" || m == "auto" }

// ---- SSE ----

func (s *Server) sse(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, unsub := s.hub.Subscribe()
	defer unsub()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

// ---- projects ----

type projectBody struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Color     string `json:"color"`
	Isolation string `json:"isolation"` // "" = detect: git repo -> worktree, otherwise direct
}

// validate checks the body and resolves the isolation mode.
func (b *projectBody) validate() string {
	if strings.TrimSpace(b.Name) == "" {
		return "nome é obrigatório"
	}
	if fi, err := os.Stat(b.Path); err != nil || !fi.IsDir() {
		return "caminho não existe ou não é uma pasta"
	}
	in := runner.Inspect(b.Path)
	switch b.Isolation {
	case "":
		b.Isolation = "direct"
		if in.IsGit && in.HasCommit {
			b.Isolation = "worktree"
		}
	case "worktree":
		if !in.IsGit || !in.HasCommit {
			return "o modo isolado precisa de um repositório git com ao menos um commit; inicie o git na pasta ou escolha \"Direto na pasta\""
		}
	case "direct":
	default:
		return "modo de isolamento inválido"
	}
	return ""
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.runner.Status())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var b struct {
		MaxParallel int `json:"maxParallel"`
	}
	if decode(r, &b) != nil || b.MaxParallel < 1 || b.MaxParallel > runner.MaxParallelCeiling {
		badRequest(w, "maxParallel deve estar entre 1 e "+strconv.Itoa(runner.MaxParallelCeiling))
		return
	}
	if err := s.store.SetSetting("max_parallel", strconv.Itoa(b.MaxParallel)); err != nil {
		fail(w, err)
		return
	}
	s.runner.SetMax(b.MaxParallel)
	writeJSON(w, 200, s.runner.Status())
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, runner.Inspect(r.URL.Query().Get("path")))
}

func (s *Server) gitInit(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Path string `json:"path"`
	}
	if decode(r, &b) != nil || b.Path == "" {
		badRequest(w, "caminho é obrigatório")
		return
	}
	if err := runner.InitRepo(b.Path); err != nil {
		badRequest(w, err.Error())
		return
	}
	writeJSON(w, 200, runner.Inspect(b.Path))
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListProjects()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, ps)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var b projectBody
	if decode(r, &b) != nil {
		badRequest(w, "JSON inválido")
		return
	}
	if msg := b.validate(); msg != "" {
		badRequest(w, msg)
		return
	}
	p, err := s.store.CreateProject(strings.TrimSpace(b.Name), b.Path, b.Color, b.Isolation)
	if err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("project", p)
	writeJSON(w, 201, p)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var b projectBody
	if decode(r, &b) != nil {
		badRequest(w, "JSON inválido")
		return
	}
	if msg := b.validate(); msg != "" {
		badRequest(w, msg)
		return
	}
	p, err := s.store.UpdateProject(pathID(r), strings.TrimSpace(b.Name), b.Path, b.Color, b.Isolation)
	if err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("project", p)
	writeJSON(w, 200, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteProject(pathID(r)); err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("project_deleted", map[string]int64{"id": pathID(r)})
	w.WriteHeader(204)
}

// ---- cards ----

type cardBody struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Column      string    `json:"column"`
	Mode        string    `json:"mode"`
	Position    float64   `json:"position"`
	Tags        *[]string `json:"tags"` // nil = leave tags untouched
}

func (s *Server) listCards(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListCards(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, cs)
}

func (s *Server) getCard(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetCard(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) createCard(w http.ResponseWriter, r *http.Request) {
	var b cardBody
	if decode(r, &b) != nil || strings.TrimSpace(b.Title) == "" {
		badRequest(w, "título é obrigatório")
		return
	}
	if b.Column != "" && !validColumn(b.Column) || b.Mode != "" && !validMode(b.Mode) {
		badRequest(w, "coluna ou modo inválido")
		return
	}
	pid := pathID(r)
	if _, err := s.store.GetProject(pid); err != nil {
		fail(w, err)
		return
	}
	c, err := s.store.CreateCard(pid, strings.TrimSpace(b.Title), b.Description, b.Column, b.Mode)
	if err != nil {
		fail(w, err)
		return
	}
	if b.Tags != nil {
		if tc, err := s.store.SetCardTitleAndTags(c.ID, "", *b.Tags); err == nil {
			c = tc
		}
	}
	s.hub.Publish("card", c)
	if c.Column == "doing" {
		s.runner.Request(c, "")
	}
	writeJSON(w, 201, c)
}

func (s *Server) updateCard(w http.ResponseWriter, r *http.Request) {
	var b cardBody
	if decode(r, &b) != nil || strings.TrimSpace(b.Title) == "" || !validMode(b.Mode) {
		badRequest(w, "título e modo (approval|auto) são obrigatórios")
		return
	}
	c, err := s.store.UpdateCard(pathID(r), strings.TrimSpace(b.Title), b.Description, b.Mode)
	if err != nil {
		fail(w, err)
		return
	}
	if b.Tags != nil {
		if tc, err := s.store.SetCardTitleAndTags(c.ID, "", *b.Tags); err == nil {
			c = tc
		}
	}
	s.hub.Publish("card", c)
	writeJSON(w, 200, c)
}

func (s *Server) moveCard(w http.ResponseWriter, r *http.Request) {
	var b cardBody
	if decode(r, &b) != nil || !validColumn(b.Column) {
		badRequest(w, "coluna inválida")
		return
	}
	prev, err := s.store.GetCard(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	c, err := s.store.MoveCard(prev.ID, b.Column, b.Position)
	if err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("card", c)
	if b.Column == "doing" && prev.Column != "doing" {
		s.runner.Request(c, "")
		c, _ = s.store.GetCard(c.ID)
		s.hub.Publish("card", c)
	}
	writeJSON(w, 200, c)
}

func (s *Server) deleteCard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if run, err := s.store.LatestRun(id); err == nil {
		_ = s.runner.Cancel(run.ID)
	}
	if err := s.store.DeleteCard(id); err != nil {
		fail(w, err)
		return
	}
	_ = os.RemoveAll(s.attachDir(id)) // path is built from the numeric id only
	s.hub.Publish("card_deleted", map[string]int64{"id": id})
	w.WriteHeader(204)
}

// runCard starts a run manually, optionally with a follow-up message (resumes the Claude session).
func (s *Server) runCard(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Message string `json:"message"`
	}
	_ = decode(r, &b)
	c, err := s.store.GetCard(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	msg := strings.TrimSpace(b.Message)
	run, err := s.runner.Request(c, msg)
	if err != nil {
		fail(w, err)
		return
	}
	if msg != "" {
		_, _ = s.comment(c.ID, "user", msg) // the follow-up is part of the card's conversation
	}
	if c.Column != "doing" {
		if mv, err := s.store.MoveCard(c.ID, "doing", c.Position); err == nil {
			s.hub.Publish("card", mv)
		}
	}
	writeJSON(w, 202, run)
}

// ---- comments ----

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListComments(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, cs)
}

// addComment stores a comment and refreshes the card (its comment count changed).
func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Body   string `json:"body"`
		Author string `json:"author"`
	}
	if decode(r, &b) != nil || strings.TrimSpace(b.Body) == "" {
		badRequest(w, "comentário vazio")
		return
	}
	c, err := s.comment(pathID(r), b.Author, strings.TrimSpace(b.Body))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) comment(cardID int64, author, body string) (store.Comment, error) {
	if _, err := s.store.GetCard(cardID); err != nil {
		return store.Comment{}, err
	}
	c, err := s.store.AddComment(cardID, author, body)
	if err != nil {
		return c, err
	}
	s.hub.Publish("comment", c)
	if card, err := s.store.GetCard(cardID); err == nil {
		s.hub.Publish("card", card)
	}
	return c, nil
}

// ---- runs ----

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	rs, err := s.store.ListRuns(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, rs)
}

func (s *Server) runEvents(w http.ResponseWriter, r *http.Request) {
	es, err := s.store.ListEvents(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, es)
}

func (s *Server) approveRun(w http.ResponseWriter, r *http.Request) {
	if err := s.runner.Approve(pathID(r)); err != nil {
		badRequest(w, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	if err := s.runner.Cancel(pathID(r)); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

// runContext loads a run with its card and project.
func (s *Server) runContext(r *http.Request) (store.Run, store.Card, store.Project, error) {
	run, err := s.store.GetRun(pathID(r))
	if err != nil {
		return run, store.Card{}, store.Project{}, err
	}
	card, err := s.store.GetCard(run.CardID)
	if err != nil {
		return run, card, store.Project{}, err
	}
	p, err := s.store.GetProject(card.ProjectID)
	return run, card, p, err
}

func (s *Server) runDiff(w http.ResponseWriter, r *http.Request) {
	run, _, p, err := s.runContext(r)
	if err != nil {
		fail(w, err)
		return
	}
	var d string
	switch {
	case p.Isolation == "direct" && run.BaseRef != "":
		var sh *runner.Shadow
		if sh, err = runner.NewShadow(s.dataDir, p.ID, p.Path); err == nil {
			d, err = sh.Diff(run.BaseRef)
		}
	case run.Branch != "":
		d, err = runner.Diff(p.Path, run.Branch)
	default:
		writeJSON(w, 200, map[string]string{"diff": ""})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"diff": d})
}

func (s *Server) mergeRun(w http.ResponseWriter, r *http.Request) {
	run, card, p, err := s.runContext(r)
	if err != nil {
		fail(w, err)
		return
	}
	if run.Status != "succeeded" {
		badRequest(w, "só é possível aceitar uma execução concluída")
		return
	}
	// direct mode: the changes are already in the folder, accepting just closes the card
	if p.Isolation != "direct" {
		if run.Branch == "" {
			badRequest(w, "execução sem branch")
			return
		}
		if err := runner.Merge(p.Path, run.Branch); err != nil {
			badRequest(w, err.Error())
			return
		}
		runner.Discard(p.Path, run.WorktreePath, run.Branch)
	}
	if mv, err := s.store.MoveCard(card.ID, "done", card.Position); err == nil {
		s.hub.Publish("card", mv)
	}
	w.WriteHeader(204)
}

func (s *Server) discardRun(w http.ResponseWriter, r *http.Request) {
	run, card, p, err := s.runContext(r)
	if err != nil {
		fail(w, err)
		return
	}
	if run.Status == "running" {
		badRequest(w, "cancele a execução antes de descartar")
		return
	}
	if p.Isolation == "direct" {
		if run.BaseRef == "" {
			badRequest(w, "não há snapshot para restaurar")
			return
		}
		sh, err := runner.NewShadow(s.dataDir, p.ID, p.Path)
		if err == nil {
			err = sh.Restore(run.BaseRef)
		}
		if err != nil {
			badRequest(w, err.Error())
			return
		}
	} else {
		runner.Discard(p.Path, run.WorktreePath, run.Branch)
	}
	if mv, err := s.store.MoveCard(card.ID, "todo", card.Position); err == nil {
		s.hub.Publish("card", mv)
	}
	w.WriteHeader(204)
}

// ---- static SPA ----

func spa(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err != nil {
			r.URL.Path = "/" // client-side routes fall back to index.html
		}
		files.ServeHTTP(w, r)
	})
}
