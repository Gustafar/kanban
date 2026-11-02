package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	_ = os.RemoveAll(s.chatAttachDir(c.ID))
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
		Text          string  `json:"text"`
		AttachmentIDs []int64 `json:"attachmentIds"`
	}
	text := ""
	if decode(r, &b) == nil {
		text = strings.TrimSpace(b.Text)
	}
	if text == "" && len(b.AttachmentIDs) == 0 {
		badRequest(w, "escreva uma mensagem")
		return
	}
	c, err := s.store.GetChat(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	if c.Title == "" { // the first message names the conversation
		title := provisionalTitle(text)
		if title == "" {
			title = "Conversa com anexos"
		}
		if c, err = s.store.UpdateChat(c.ID, title, c.Model, c.Effort); err != nil {
			fail(w, err)
			return
		}
	}
	if err := s.runner.SendChat(c, text, b.AttachmentIDs); err != nil {
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

func (s *Server) chatAttachDir(chatID int64) string {
	return filepath.Join(s.dataDir, "attachments", "chat-"+strconv.FormatInt(chatID, 10))
}

// addChatAttachment takes the raw file as the request body (name in ?name=), like card attachments.
// It stays unlinked until a message that references its id is sent.
func (s *Server) addChatAttachment(w http.ResponseWriter, r *http.Request) {
	chatID := pathID(r)
	if _, err := s.store.GetChat(chatID); err != nil {
		fail(w, err)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttachment))
	if err != nil || len(data) == 0 {
		badRequest(w, "arquivo vazio ou maior que 15 MB")
		return
	}
	name := safeName(r.URL.Query().Get("name"))
	dir := s.chatAttachDir(chatID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(w, err)
		return
	}
	prefix := make([]byte, 4)
	_, _ = rand.Read(prefix)
	path := filepath.Join(dir, hex.EncodeToString(prefix)+"-"+name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fail(w, err)
		return
	}
	a, err := s.store.AddChatAttachment(chatID, name, http.DetectContentType(data), int64(len(data)), path)
	if err != nil {
		_ = os.Remove(path)
		fail(w, err)
		return
	}
	writeJSON(w, 201, a)
}

func (s *Server) serveChatAttachment(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetChatAttachment(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	mime := strings.SplitN(a.Mime, ";", 2)[0]
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	if inlineTypes[mime] {
		w.Header().Set("Content-Type", mime)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+a.Name+`"`)
	}
	http.ServeFile(w, r, a.Path)
}

// deleteChatAttachment only removes files not yet sent: sent ones are part of the history.
func (s *Server) deleteChatAttachment(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetChatAttachment(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	if a.MessageID != 0 {
		badRequest(w, "o arquivo já foi enviado na conversa")
		return
	}
	if err := s.store.DeleteChatAttachment(a.ID); err != nil {
		fail(w, err)
		return
	}
	_ = os.Remove(a.Path)
	w.WriteHeader(204)
}
