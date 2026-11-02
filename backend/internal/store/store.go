package store

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

var Columns = []string{"backlog", "todo", "doing", "review", "done"}

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Color     string    `json:"color"`
	Isolation string    `json:"isolation"` // "worktree" (git, isolated copy) | "direct" (edit the folder, shadow snapshots)
	CreatedAt time.Time `json:"createdAt"`
}

type Card struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"projectId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Column      string    `json:"column"`
	Position    float64   `json:"position"`
	Mode        string    `json:"mode"` // "approval" | "auto"
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	RunStatus   string    `json:"runStatus"` // status of latest run, "" if none
	Comments    int       `json:"commentCount"`
	Tags        []string  `json:"tags"`
	Attachments int       `json:"attachmentCount"`
	Titling     bool      `json:"titling"`   // the AI is still naming/tagging this card (in memory only)
	Model       string    `json:"model"`     // "" = the CLI's default; otherwise an alias like opus, sonnet, haiku, fable
	Effort      string    `json:"effort"`    // "" = default; low | medium | high | xhigh | max
	Polishing   bool      `json:"polishing"` // the AI is discussing/refining this card (in memory only)
	DependsOn   int64     `json:"dependsOn"` // 0 = none; otherwise a card that must reach "done" before this one can run
}

type Attachment struct {
	ID        int64     `json:"id"`
	CardID    int64     `json:"cardId"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"` // absolute path on disk, handed to the AI so it can read the file
	CreatedAt time.Time `json:"createdAt"`
}

// NormalizeTags lowercases, strips '#', ',' and spaces at the edges, drops empties/duplicates and caps the list.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.Trim(strings.TrimSpace(strings.ReplaceAll(t, ",", " ")), "# "))
		if r := []rune(t); len(r) > 24 {
			t = string(r[:24])
		}
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == 6 {
			break
		}
	}
	return out
}

type Comment struct {
	ID        int64     `json:"id"`
	CardID    int64     `json:"cardId"`
	Author    string    `json:"author"` // "user" | "ai"
	Body      string    `json:"body"`
	RunID     int64     `json:"runId"` // 0 = not tied to a diff line
	File      string    `json:"file"`  // diff file path, set together with Line
	Line      int       `json:"line"`  // 1-based line within the diff's new (or old, for deletions) file; 0 = not a line comment
	CreatedAt time.Time `json:"createdAt"`
}

type Run struct {
	ID            int64      `json:"id"`
	CardID        int64      `json:"cardId"`
	Status        string     `json:"status"` // awaiting_approval | running | succeeded | failed | cancelled
	SessionID     string     `json:"sessionId"`
	Branch        string     `json:"branch"`
	WorktreePath  string     `json:"worktreePath"`
	BaseRef       string     `json:"baseRef"` // direct mode: shadow commit taken before the run
	Prompt        string     `json:"prompt"`
	Error         string     `json:"error"`
	CostUSD       float64    `json:"costUsd"`
	InputTokens   int64      `json:"inputTokens"`
	OutputTokens  int64      `json:"outputTokens"`
	CacheReadTok  int64      `json:"cacheReadTokens"`
	CacheWriteTok int64      `json:"cacheCreationTokens"`
	StartedAt     *time.Time `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// Usage is an aggregate of Claude Code cost/token usage across runs.
type Usage struct {
	CostUSD       float64 `json:"costUsd"`
	InputTokens   int64   `json:"inputTokens"`
	OutputTokens  int64   `json:"outputTokens"`
	CacheReadTok  int64   `json:"cacheReadTokens"`
	CacheWriteTok int64   `json:"cacheCreationTokens"`
	Runs          int64   `json:"runs"`
}

// ProjectUsage is a project's share of the total usage.
type ProjectUsage struct {
	ProjectID int64  `json:"projectId"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Usage
}

// DailyUsage is one day's usage, for a simple trend chart.
type DailyUsage struct {
	Date string `json:"date"` // YYYY-MM-DD
	Usage
}

type RunEvent struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"runId"`
	Kind      string    `json:"kind"` // text | tool | result | error | system
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store struct {
	db        *sql.DB
	titling   sync.Map // cardID -> struct{}: cards the AI is naming right now
	polishing sync.Map // cardID -> struct{}: cards the AI is discussing right now
}

// SetTitling marks (or unmarks) a card as being named by the AI.
func (s *Store) SetTitling(cardID int64, on bool) {
	if on {
		s.titling.Store(cardID, struct{}{})
	} else {
		s.titling.Delete(cardID)
	}
}

func (s *Store) isTitling(cardID int64) bool {
	_, ok := s.titling.Load(cardID)
	return ok
}

// SetPolishing marks (or unmarks) a card as being discussed/refined by the AI before it runs.
func (s *Store) SetPolishing(cardID int64, on bool) {
	if on {
		s.polishing.Store(cardID, struct{}{})
	} else {
		s.polishing.Delete(cardID)
	}
}

func (s *Store) isPolishing(cardID int64) bool {
	_, ok := s.polishing.Load(cardID)
	return ok
}

// IsPolishing reports whether the AI is currently discussing this card (see SetPolishing).
func (s *Store) IsPolishing(cardID int64) bool { return s.isPolishing(cardID) }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if _, err := db.Exec(schema + chatSchema); err != nil {
		return nil, err
	}
	// additive migrations for databases created by earlier versions
	for _, stmt := range []string{
		`ALTER TABLE projects ADD COLUMN isolation TEXT NOT NULL DEFAULT 'worktree'`,
		`ALTER TABLE runs ADD COLUMN base_ref TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cards ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cards ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cards ADD COLUMN effort TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE runs ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE runs ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE runs ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE runs ADD COLUMN cache_creation_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cards ADD COLUMN depends_on INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE comments ADD COLUMN run_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE comments ADD COLUMN file TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE comments ADD COLUMN line INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	path TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#6366f1',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS cards (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	col TEXT NOT NULL DEFAULT 'backlog',
	position REAL NOT NULL DEFAULT 0,
	mode TEXT NOT NULL DEFAULT 'approval',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
	status TEXT NOT NULL,
	session_id TEXT NOT NULL DEFAULT '',
	branch TEXT NOT NULL DEFAULT '',
	worktree_path TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	started_at DATETIME,
	finished_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS run_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	data TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
	author TEXT NOT NULL,
	body TEXT NOT NULL,
	run_id INTEGER NOT NULL DEFAULT 0,
	file TEXT NOT NULL DEFAULT '',
	line INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS attachments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	mime TEXT NOT NULL,
	size INTEGER NOT NULL,
	path TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_attachments_card ON attachments(card_id);
CREATE INDEX IF NOT EXISTS idx_comments_card ON comments(card_id);
CREATE INDEX IF NOT EXISTS idx_cards_project ON cards(project_id);
CREATE INDEX IF NOT EXISTS idx_runs_card ON runs(card_id);
CREATE INDEX IF NOT EXISTS idx_events_run ON run_events(run_id);
`

// ---- projects ----

func (s *Store) CreateProject(name, path, color, isolation string) (Project, error) {
	if isolation != "direct" {
		isolation = "worktree"
	}
	if color == "" {
		// rotate through the UI swatches so new projects are told apart at a glance
		palette := []string{"#3355FF", "#23915F", "#D9930D", "#D2425A", "#8A4FFF", "#0F9CB8", "#E2662B"}
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n)
		color = palette[n%len(palette)]
	}
	res, err := s.db.Exec(`INSERT INTO projects(name,path,color,isolation) VALUES(?,?,?,?)`, name, path, color, isolation)
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetProject(id)
}

func (s *Store) GetProject(id int64) (Project, error) {
	var p Project
	err := s.db.QueryRow(`SELECT id,name,path,color,isolation,created_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &p.Color, &p.Isolation, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id,name,path,color,isolation,created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.Color, &p.Isolation, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(id int64, name, path, color, isolation string) (Project, error) {
	if isolation != "direct" {
		isolation = "worktree"
	}
	res, err := s.db.Exec(`UPDATE projects SET name=?,path=?,color=?,isolation=? WHERE id=?`, name, path, color, isolation, id)
	if err != nil {
		return Project{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Project{}, ErrNotFound
	}
	return s.GetProject(id)
}

func (s *Store) DeleteProject(id int64) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

// ---- cards ----

const cardSelect = `SELECT c.id,c.project_id,c.title,c.description,c.col,c.position,c.mode,c.created_at,c.updated_at,
	COALESCE((SELECT status FROM runs r WHERE r.card_id=c.id ORDER BY r.id DESC LIMIT 1),''),
	(SELECT COUNT(*) FROM comments m WHERE m.card_id=c.id),
	c.tags,
	(SELECT COUNT(*) FROM attachments a WHERE a.card_id=c.id),
	c.model, c.effort, c.depends_on
	FROM cards c`

func scanCard(sc interface{ Scan(...any) error }) (Card, error) {
	var c Card
	var tags string
	err := sc.Scan(&c.ID, &c.ProjectID, &c.Title, &c.Description, &c.Column, &c.Position, &c.Mode, &c.CreatedAt, &c.UpdatedAt, &c.RunStatus, &c.Comments, &tags, &c.Attachments, &c.Model, &c.Effort, &c.DependsOn)
	c.Tags = []string{}
	if tags != "" {
		c.Tags = strings.Split(tags, ",")
	}
	return c, err
}

func (s *Store) CreateCard(projectID int64, title, description, column, mode string) (Card, error) {
	if column == "" {
		column = "backlog"
	}
	if mode == "" {
		mode = "approval"
	}
	var maxPos sql.NullFloat64
	_ = s.db.QueryRow(`SELECT MAX(position) FROM cards WHERE project_id=? AND col=?`, projectID, column).Scan(&maxPos)
	res, err := s.db.Exec(`INSERT INTO cards(project_id,title,description,col,position,mode) VALUES(?,?,?,?,?,?)`,
		projectID, title, description, column, maxPos.Float64+1024, mode)
	if err != nil {
		return Card{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetCard(id)
}

func (s *Store) GetCard(id int64) (Card, error) {
	c, err := scanCard(s.db.QueryRow(cardSelect+` WHERE c.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	c.Titling = s.isTitling(c.ID)
	c.Polishing = s.isPolishing(c.ID)
	return c, err
}

func (s *Store) ListCards(projectID int64) ([]Card, error) {
	rows, err := s.db.Query(cardSelect+` WHERE c.project_id=? ORDER BY c.position`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		c.Titling = s.isTitling(c.ID)
		c.Polishing = s.isPolishing(c.ID)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateCard(id int64, title, description, mode string) (Card, error) {
	res, err := s.db.Exec(`UPDATE cards SET title=?,description=?,mode=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, description, mode, id)
	if err != nil {
		return Card{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Card{}, ErrNotFound
	}
	return s.GetCard(id)
}

// SetCardModel stores which model and effort level the card's runs use ("" = the CLI default).
func (s *Store) SetCardModel(id int64, model, effort string) (Card, error) {
	if _, err := s.db.Exec(`UPDATE cards SET model=?,effort=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, model, effort, id); err != nil {
		return Card{}, err
	}
	return s.GetCard(id)
}

// SetCardDependency sets which card (0 = none) must reach the "done" column before this one's runs can start.
func (s *Store) SetCardDependency(id, dependsOn int64) (Card, error) {
	if _, err := s.db.Exec(`UPDATE cards SET depends_on=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, dependsOn, id); err != nil {
		return Card{}, err
	}
	return s.GetCard(id)
}

func (s *Store) SetCardTitleAndTags(id int64, title string, tags []string) (Card, error) {
	_, err := s.db.Exec(`UPDATE cards SET title=COALESCE(NULLIF(?,''),title),tags=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		title, strings.Join(NormalizeTags(tags), ","), id)
	if err != nil {
		return Card{}, err
	}
	return s.GetCard(id)
}

func (s *Store) MoveCard(id int64, column string, position float64) (Card, error) {
	res, err := s.db.Exec(`UPDATE cards SET col=?,position=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, column, position, id)
	if err != nil {
		return Card{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Card{}, ErrNotFound
	}
	return s.GetCard(id)
}

func (s *Store) DeleteCard(id int64) error {
	_, err := s.db.Exec(`DELETE FROM cards WHERE id=?`, id)
	return err
}

// ---- runs ----

const runSelect = `SELECT id,card_id,status,session_id,branch,worktree_path,base_ref,prompt,error,
	cost_usd,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,started_at,finished_at,created_at FROM runs`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	err := sc.Scan(&r.ID, &r.CardID, &r.Status, &r.SessionID, &r.Branch, &r.WorktreePath, &r.BaseRef, &r.Prompt, &r.Error,
		&r.CostUSD, &r.InputTokens, &r.OutputTokens, &r.CacheReadTok, &r.CacheWriteTok, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	return r, err
}

func (s *Store) CreateRun(cardID int64, status, prompt string) (Run, error) {
	res, err := s.db.Exec(`INSERT INTO runs(card_id,status,prompt) VALUES(?,?,?)`, cardID, status, prompt)
	if err != nil {
		return Run{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetRun(id)
}

func (s *Store) GetRun(id int64) (Run, error) {
	r, err := scanRun(s.db.QueryRow(runSelect+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) LatestRun(cardID int64) (Run, error) {
	r, err := scanRun(s.db.QueryRow(runSelect+` WHERE card_id=? ORDER BY id DESC LIMIT 1`, cardID))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) ListRuns(cardID int64) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE card_id=? ORDER BY id DESC`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateRun applies a partial update; zero-valued strings are left untouched.
func (s *Store) UpdateRun(id int64, status, sessionID, branch, worktree, errMsg string, started, finished *time.Time) error {
	_, err := s.db.Exec(`UPDATE runs SET
		status=COALESCE(NULLIF(?,''),status),
		session_id=COALESCE(NULLIF(?,''),session_id),
		branch=COALESCE(NULLIF(?,''),branch),
		worktree_path=COALESCE(NULLIF(?,''),worktree_path),
		error=COALESCE(NULLIF(?,''),error),
		started_at=COALESCE(?,started_at),
		finished_at=COALESCE(?,finished_at)
		WHERE id=?`, status, sessionID, branch, worktree, errMsg, started, finished, id)
	return err
}

// ResetInterruptedRuns resets runs left "running" by a previous server process back to "queued"
// (the run queue itself lives only in memory and does not survive a restart), then returns every
// run now waiting to start — "queued" runs included — in creation order, so the caller can requeue them.
func (s *Store) ResetInterruptedRuns() ([]Run, error) {
	if _, err := s.db.Exec(`UPDATE runs SET status='queued' WHERE status='running'`); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(runSelect + ` WHERE status='queued' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetSetting(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) AddEvent(runID int64, kind, data string) (RunEvent, error) {
	res, err := s.db.Exec(`INSERT INTO run_events(run_id,kind,data) VALUES(?,?,?)`, runID, kind, data)
	if err != nil {
		return RunEvent{}, err
	}
	id, _ := res.LastInsertId()
	return RunEvent{ID: id, RunID: runID, Kind: kind, Data: data, CreatedAt: time.Now()}, nil
}

func (s *Store) ListEvents(runID int64) ([]RunEvent, error) {
	rows, err := s.db.Query(`SELECT id,run_id,kind,data,created_at FROM run_events WHERE run_id=? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		var e RunEvent
		if err := rows.Scan(&e.ID, &e.RunID, &e.Kind, &e.Data, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) AddAttachment(cardID int64, name, mime string, size int64, path string) (Attachment, error) {
	res, err := s.db.Exec(`INSERT INTO attachments(card_id,name,mime,size,path) VALUES(?,?,?,?,?)`, cardID, name, mime, size, path)
	if err != nil {
		return Attachment{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetAttachment(id)
}

func (s *Store) GetAttachment(id int64) (Attachment, error) {
	var a Attachment
	err := s.db.QueryRow(`SELECT id,card_id,name,mime,size,path,created_at FROM attachments WHERE id=?`, id).
		Scan(&a.ID, &a.CardID, &a.Name, &a.Mime, &a.Size, &a.Path, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) ListAttachments(cardID int64) ([]Attachment, error) {
	rows, err := s.db.Query(`SELECT id,card_id,name,mime,size,path,created_at FROM attachments WHERE card_id=? ORDER BY id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.CardID, &a.Name, &a.Mime, &a.Size, &a.Path, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAttachment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM attachments WHERE id=?`, id)
	return err
}

func (s *Store) AddComment(cardID int64, author, body string) (Comment, error) {
	return s.addComment(cardID, author, body, 0, "", 0)
}

// AddLineComment attaches a comment to a specific file/line of a run's diff, for review feedback.
func (s *Store) AddLineComment(cardID int64, author, body string, runID int64, file string, line int) (Comment, error) {
	return s.addComment(cardID, author, body, runID, file, line)
}

func (s *Store) addComment(cardID int64, author, body string, runID int64, file string, line int) (Comment, error) {
	if author != "ai" {
		author = "user"
	}
	res, err := s.db.Exec(`INSERT INTO comments(card_id,author,body,run_id,file,line) VALUES(?,?,?,?,?,?)`, cardID, author, body, runID, file, line)
	if err != nil {
		return Comment{}, err
	}
	id, _ := res.LastInsertId()
	return Comment{ID: id, CardID: cardID, Author: author, Body: body, RunID: runID, File: file, Line: line, CreatedAt: time.Now()}, nil
}

func (s *Store) ListComments(cardID int64) ([]Comment, error) {
	rows, err := s.db.Query(`SELECT id,card_id,author,body,run_id,file,line,created_at FROM comments WHERE card_id=? ORDER BY id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.CardID, &c.Author, &c.Body, &c.RunID, &c.File, &c.Line, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SetRunBaseRef(id int64, ref string) error {
	_, err := s.db.Exec(`UPDATE runs SET base_ref=? WHERE id=?`, ref, id)
	return err
}

// SetRunUsage stores the cost/token totals Claude Code reported for a run.
func (s *Store) SetRunUsage(id int64, costUSD float64, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64) error {
	_, err := s.db.Exec(`UPDATE runs SET cost_usd=?,input_tokens=?,output_tokens=?,cache_read_tokens=?,cache_creation_tokens=? WHERE id=?`,
		costUSD, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens, id)
	return err
}

// scanUsage scans a row whose columns are [leading..., cost, inputTok, outputTok, cacheRead, cacheWrite, count].
func scanUsage(sc interface{ Scan(...any) error }, leading ...any) (Usage, error) {
	var u Usage
	args := append(append([]any{}, leading...), &u.CostUSD, &u.InputTokens, &u.OutputTokens, &u.CacheReadTok, &u.CacheWriteTok, &u.Runs)
	err := sc.Scan(args...)
	return u, err
}

const usageSelect = `COALESCE(SUM(cost_usd),0),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),
	COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(cache_creation_tokens),0),COUNT(*)`

// GetUsage totals Claude Code cost/tokens across every run that reported them.
func (s *Store) GetUsage() (Usage, error) {
	return scanUsage(s.db.QueryRow(`SELECT ` + usageSelect + ` FROM runs WHERE cost_usd > 0 OR input_tokens > 0 OR output_tokens > 0`))
}

// UsageByProject breaks the totals down per project, highest cost first.
func (s *Store) UsageByProject() ([]ProjectUsage, error) {
	rows, err := s.db.Query(`SELECT p.id,p.name,p.color,` + usageSelect + `
		FROM runs r JOIN cards c ON c.id=r.card_id JOIN projects p ON p.id=c.project_id
		WHERE r.cost_usd > 0 OR r.input_tokens > 0 OR r.output_tokens > 0
		GROUP BY p.id ORDER BY SUM(r.cost_usd) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectUsage{}
	for rows.Next() {
		var pu ProjectUsage
		u, err := scanUsage(rows, &pu.ProjectID, &pu.Name, &pu.Color)
		if err != nil {
			return nil, err
		}
		pu.Usage = u
		out = append(out, pu)
	}
	return out, rows.Err()
}

// UsageByDay breaks the totals down per day (run creation date), most recent first, capped at n days.
func (s *Store) UsageByDay(n int) ([]DailyUsage, error) {
	rows, err := s.db.Query(`SELECT date(created_at),`+usageSelect+`
		FROM runs WHERE cost_usd > 0 OR input_tokens > 0 OR output_tokens > 0
		GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DailyUsage{}
	for rows.Next() {
		var du DailyUsage
		u, err := scanUsage(rows, &du.Date)
		if err != nil {
			return nil, err
		}
		du.Usage = u
		out = append(out, du)
	}
	return out, rows.Err()
}
