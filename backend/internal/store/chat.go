package store

import (
	"database/sql"
	"errors"
	"time"
)

// A chat is a conversation between the user and the AI about one project's board (not tied to a card).
type Chat struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"projectId"`
	Title     string    `json:"title"`
	SessionID string    `json:"-"` // Claude CLI session, for --resume
	Model     string    `json:"model"`
	Effort    string    `json:"effort"`
	Busy      bool      `json:"busy"` // filled in by the API: the AI is answering right now
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ChatMessage struct {
	ID        int64     `json:"id"`
	ChatID    int64     `json:"chatId"`
	Role      string    `json:"role"` // user | assistant | tool | error
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

const chatSchema = `
CREATE TABLE IF NOT EXISTS chats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	title TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	effort TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chat_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
	role TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chats_project ON chats(project_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_chat ON chat_messages(chat_id);
`

const chatSelect = `SELECT id,project_id,title,session_id,model,effort,created_at,updated_at FROM chats`

func scanChat(row interface{ Scan(...any) error }) (Chat, error) {
	var c Chat
	err := row.Scan(&c.ID, &c.ProjectID, &c.Title, &c.SessionID, &c.Model, &c.Effort, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateChat(projectID int64, model, effort string) (Chat, error) {
	res, err := s.db.Exec(`INSERT INTO chats(project_id,model,effort) VALUES(?,?,?)`, projectID, model, effort)
	if err != nil {
		return Chat{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetChat(id)
}

func (s *Store) GetChat(id int64) (Chat, error) {
	return scanChat(s.db.QueryRow(chatSelect+` WHERE id=?`, id))
}

// ListChats returns the project's conversations, most recently active first.
func (s *Store) ListChats(projectID int64) ([]Chat, error) {
	rows, err := s.db.Query(chatSelect+` WHERE project_id=? ORDER BY updated_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Chat{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateChat(id int64, title, model, effort string) (Chat, error) {
	if _, err := s.db.Exec(`UPDATE chats SET title=?,model=?,effort=? WHERE id=?`, title, model, effort, id); err != nil {
		return Chat{}, err
	}
	return s.GetChat(id)
}

func (s *Store) SetChatSession(id int64, session string) error {
	_, err := s.db.Exec(`UPDATE chats SET session_id=? WHERE id=?`, session, id)
	return err
}

func (s *Store) DeleteChat(id int64) error {
	_, err := s.db.Exec(`DELETE FROM chats WHERE id=?`, id)
	return err
}

func (s *Store) AddChatMessage(chatID int64, role, body string) (ChatMessage, error) {
	res, err := s.db.Exec(`INSERT INTO chat_messages(chat_id,role,body) VALUES(?,?,?)`, chatID, role, body)
	if err != nil {
		return ChatMessage{}, err
	}
	_, _ = s.db.Exec(`UPDATE chats SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, chatID)
	id, _ := res.LastInsertId()
	return ChatMessage{ID: id, ChatID: chatID, Role: role, Body: body, CreatedAt: time.Now()}, nil
}

func (s *Store) ListChatMessages(chatID int64) ([]ChatMessage, error) {
	rows, err := s.db.Query(`SELECT id,chat_id,role,body,created_at FROM chat_messages WHERE chat_id=? ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ChatID, &m.Role, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
