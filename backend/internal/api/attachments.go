package api

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const maxAttachment = 15 << 20 // 15 MB

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// inlineTypes are the only types the browser may render in place; everything else downloads.
var inlineTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

func (s *Server) attachDir(cardID int64) string {
	return filepath.Join(s.dataDir, "attachments", "card-"+strconv.FormatInt(cardID, 10))
}

func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Trim(unsafeName.ReplaceAllString(name, "_"), " ._")
	if name == "" {
		name = "arquivo"
	}
	if r := []rune(name); len(r) > 80 {
		name = string(r[len(r)-80:])
	}
	return name
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	as, err := s.store.ListAttachments(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, as)
}

// addAttachment takes the raw file as the request body (name in ?name=). The type is sniffed, not trusted.
func (s *Server) addAttachment(w http.ResponseWriter, r *http.Request) {
	cardID := pathID(r)
	if _, err := s.store.GetCard(cardID); err != nil {
		fail(w, err)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttachment))
	if err != nil || len(data) == 0 {
		badRequest(w, "arquivo vazio ou maior que 15 MB")
		return
	}
	name := safeName(r.URL.Query().Get("name"))
	dir := s.attachDir(cardID)
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
	a, err := s.store.AddAttachment(cardID, name, http.DetectContentType(data), int64(len(data)), path)
	if err != nil {
		_ = os.Remove(path)
		fail(w, err)
		return
	}
	s.hub.Publish("attachment", a)
	s.republishCard(cardID)
	writeJSON(w, 201, a)
}

func (s *Server) serveAttachment(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAttachment(pathID(r))
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

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAttachment(pathID(r))
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteAttachment(a.ID); err != nil {
		fail(w, err)
		return
	}
	_ = os.Remove(a.Path)
	s.hub.Publish("attachment_deleted", map[string]int64{"id": a.ID, "cardId": a.CardID})
	s.republishCard(a.CardID)
	w.WriteHeader(204)
}

func (s *Server) republishCard(id int64) {
	if c, err := s.store.GetCard(id); err == nil {
		s.hub.Publish("card", c)
	}
}
