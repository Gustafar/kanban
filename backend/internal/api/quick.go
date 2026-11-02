package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"kanban_board/internal/titler"
)

// provisionalTitle is what the card shows until the AI title arrives.
func provisionalTitle(text string) string {
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	if r := []rune(first); len(r) > 60 {
		first = strings.TrimSpace(string(r[:60])) + "…"
	}
	return first
}

// quickCard creates a card straight from free text: the card exists immediately,
// then the AI replaces the provisional title and adds tags in the background.
func (s *Server) quickCard(w http.ResponseWriter, r *http.Request) {
	var b cardBody
	if decode(r, &b) != nil || strings.TrimSpace(b.Description) == "" {
		badRequest(w, "escreva o que você quer fazer")
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
	text := strings.TrimSpace(b.Description)
	c, err := s.store.CreateCard(pid, provisionalTitle(text), text, b.Column, b.Mode)
	if err != nil {
		fail(w, err)
		return
	}
	s.store.SetTitling(c.ID, true)
	c.Titling = true
	s.hub.Publish("card", c)
	if c.Column == "doing" {
		s.runner.Request(c, "")
	}
	go func() {
		title, tags, err := titler.Generate(context.Background(), text)
		s.store.SetTitling(c.ID, false)
		if err != nil {
			log.Printf("título automático do card #%d falhou: %v", c.ID, err)
			s.republishCard(c.ID) // clears the "naming…" state
			s.hub.Publish("notify", map[string]any{"kind": "warn", "title": "A IA não conseguiu nomear o card",
				"body": "Ficou com o título provisório. Dá para renomear na gaveta.", "cardId": c.ID})
			return
		}
		// the card may have been deleted or renamed by hand meanwhile: only replace the provisional title
		cur, err := s.store.GetCard(c.ID)
		if err != nil {
			return
		}
		if cur.Title != c.Title {
			title = ""
		}
		if updated, err := s.store.SetCardTitleAndTags(c.ID, title, tags); err == nil {
			s.hub.Publish("card", updated)
		}
	}()
	writeJSON(w, 201, c)
}
