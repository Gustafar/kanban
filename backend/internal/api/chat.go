package api

import (
	"errors"
	"net/http"
	"strings"

	"kanban_board/internal/runner"
	"kanban_board/internal/store"
)

type chatBody struct {
	Title  *string `json:"title"`
	Model  *string `json:"model"`
	Effort *string `json:"effort"`
}

// withBusy fills in whether the AI is answering in the chat right now.
func (s *Server) withBusy(c store.Chat) store.Chat {
	c.Busy = s.runner.ChatBusy(c.ID)
	return c
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListChats(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	for i := range cs {
		cs[i] = s.withBusy(cs[i])
	}
	writeJSON(w, 200, cs)
}

func (s *Server) createChat(w http.ResponseWriter, r *http.Request) {
	var b chatBody
	_ = decode(r, &b) // the body is optional
	pid := pathID(r)
	if _, err := s.store.GetProject(pid); err != nil {
		fail(w, err)
		return
	}
	model, effort := "", ""
	if b.Model != nil {
		model = strings.TrimSpace(*b.Model)
	}
	if b.Effort != nil {
		effort = strings.TrimSpace(*b.Effort)
	}
	if !validModel(model) || !validEffort(effort) {
		badRequest(w, "modelo ou esforço inválido")
		return
	}
	c, err := s.store.CreateChat(pid, model, effort)
	if err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("chat", c)
	writeJSON(w, 201, c)
}

func (s *Server) updateChat(w http.ResponseWriter, r *http.Request) {
	var b chatBody
	if decode(r, &b) != nil {
		badRequest(w, "corpo inválido")
		return
	}
	c, err := s.store.GetChat(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	title, model, effort := c.Title, c.Model, c.Effort // each field may be sent alone
	if b.Title != nil {
		title = strings.TrimSpace(*b.Title)
	}
	if b.Model != nil {
		model = strings.TrimSpace(*b.Model)
	}
	if b.Effort != nil {
		effort = strings.TrimSpace(*b.Effort)
	}
	if !validModel(model) || !validEffort(effort) {
		badRequest(w, "modelo ou esforço inválido")
		return
	}
	c, err = s.store.UpdateChat(c.ID, title, model, effort)
	if err != nil {
		fail(w, err)
		return
	}
	c = s.withBusy(c)
	s.hub.Publish("chat", c)
	writeJSON(w, 200, c)
}

func (s *Server) deleteChat(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetChat(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	s.runner.CancelChat(c.ID)
	if err := s.store.DeleteChat(c.ID); err != nil {
		fail(w, err)
		return
	}
	s.hub.Publish("chat_deleted", map[string]int64{"id": c.ID, "projectId": c.ProjectID})
	w.WriteHeader(204)
}

func (s *Server) listChatMessages(w http.ResponseWriter, r *http.Request) {
	ms, err := s.store.ListChatMessages(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, ms)
}

func (s *Server) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Text string `json:"text"`
	}
	text := ""
	if decode(r, &b) == nil {
		text = strings.TrimSpace(b.Text)
	}
	if text == "" {
		badRequest(w, "escreva uma mensagem")
		return
	}
	c, err := s.store.GetChat(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	if c.Title == "" { // the first message names the conversation
		if c, err = s.store.UpdateChat(c.ID, provisionalTitle(text), c.Model, c.Effort); err != nil {
			fail(w, err)
			return
		}
	}
	if err := s.runner.SendChat(c, text); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, runner.ErrChatBusy) {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(202)
}

func (s *Server) cancelChat(w http.ResponseWriter, r *http.Request) {
	s.runner.CancelChat(pathID(r))
	w.WriteHeader(204)
}
