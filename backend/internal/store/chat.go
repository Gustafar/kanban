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

	Attachments []ChatAttachment `json:"attachments"`
}

// ChatAttachment is a file sent in a chat message. MessageID is 0 until the message is sent.
type ChatAttachment struct {
	ID        int64     `json:"id"`
	ChatID    int64     `json:"chatId"`
	MessageID int64     `json:"messageId"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Path      string    `json:"-"`
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
CREATE TABLE IF NOT EXISTS chat_attachments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
	message_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	mime TEXT NOT NULL,
	size INTEGER NOT NULL,
	path TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chat_attachments_chat ON chat_attachments(chat_id);
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
	return ChatMessage{ID: id, ChatID: chatID, Role: role, Body: body, CreatedAt: time.Now(), Attachments: []ChatAttachment{}}, nil
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
		m.Attachments = []ChatAttachment{}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // the single connection must be free for the next query
	files, err := s.ListChatAttachments(chatID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*ChatMessage{}
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	for _, a := range files {
		if m := byID[a.MessageID]; m != nil {
			m.Attachments = append(m.Attachments, a)
		}
	}
	return out, nil
}

const chatAttSelect = `SELECT id,chat_id,message_id,name,mime,size,path,created_at FROM chat_attachments`

func scanChatAttachment(row interface{ Scan(...any) error }) (ChatAttachment, error) {
	var a ChatAttachment
	err := row.Scan(&a.ID, &a.ChatID, &a.MessageID, &a.Name, &a.Mime, &a.Size, &a.Path, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) AddChatAttachment(chatID int64, name, mime string, size int64, path string) (ChatAttachment, error) {
	res, err := s.db.Exec(`INSERT INTO chat_attachments(chat_id,name,mime,size,path) VALUES(?,?,?,?,?)`, chatID, name, mime, size, path)
	if err != nil {
		return ChatAttachment{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetChatAttachment(id)
}

func (s *Store) GetChatAttachment(id int64) (ChatAttachment, error) {
	return scanChatAttachment(s.db.QueryRow(chatAttSelect+` WHERE id=?`, id))
}

func (s *Store) ListChatAttachments(chatID int64) ([]ChatAttachment, error) {
	rows, err := s.db.Query(chatAttSelect+` WHERE chat_id=? ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatAttachment{}
	for rows.Next() {
		a, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LinkChatAttachments ties files uploaded earlier to the message that carries them.
func (s *Store) LinkChatAttachments(messageID int64, ids []int64) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE chat_attachments SET message_id=? WHERE id=?`, messageID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteChatAttachment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM chat_attachments WHERE id=?`, id)
	return err
}
