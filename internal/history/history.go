// Package history manages a SQLite-backed audit log of all requests, intent
// parses, rendered commands, and executions. All stored data is redacted.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)

	"github.com/slim/cmdctx/internal/config"
	"github.com/slim/cmdctx/internal/utils"
)

// Entry is a single history record.
type Entry struct {
	ID            int64     `json:"id"`
	Prompt        string    `json:"prompt"`
	IntentType    string    `json:"intent_type"`
	IntentJSON    string    `json:"intent_json"`
	RenderedCmd   string    `json:"rendered_cmd"`
	ParsedBy      string    `json:"parsed_by"` // "ai" | "rule_based"
	Risk          string    `json:"risk"`
	Cwd           string    `json:"cwd"`
	Executed      bool      `json:"executed"`
	ExitCode      *int      `json:"exit_code,omitempty"`
	DurationMS    int64     `json:"duration_ms,omitempty"`
	StdoutSnippet string    `json:"stdout_snippet,omitempty"`
	StderrSnippet string    `json:"stderr_snippet,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Store is the history database.
type Store struct {
	db *sql.DB
}

// DBPath returns the path to the history database file.
func DBPath() string {
	return filepath.Join(config.GlobalDir(), "history.db")
}

// Open opens (or creates) the SQLite history database.
func Open() (*Store, error) {
	if err := os.MkdirAll(config.GlobalDir(), 0o700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}
	return openDB(DBPath())
}

// openDB opens a SQLite database at the given path and runs migrations.
// It is exported at package level (lowercase) so tests can use it with a temp path.
func openDB(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening history DB: %w", err)
	}

	// Enforce single-writer mode for SQLite safety.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating history DB: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Record inserts a new history entry. All string fields are redacted before storage.
func (s *Store) Record(e *Entry) (int64, error) {
	// Redact before storing — safety first.
	prompt := utils.RedactString(e.Prompt)
	renderedCmd := utils.RedactString(e.RenderedCmd)
	stdoutSnippet := utils.RedactString(e.StdoutSnippet)
	stderrSnippet := utils.RedactString(e.StderrSnippet)
	intentJSON := utils.RedactString(e.IntentJSON)

	cwd := e.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	now := time.Now().UTC()
	res, err := s.db.Exec(`
		INSERT INTO history
			(prompt, intent_type, intent_json, rendered_cmd, parsed_by, risk, cwd,
			 executed, exit_code, duration_ms, stdout_snippet, stderr_snippet, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
	`,
		prompt, e.IntentType, intentJSON, renderedCmd, e.ParsedBy, e.Risk, cwd,
		boolToInt(e.Executed), e.ExitCode, e.DurationMS,
		stdoutSnippet, stderrSnippet, now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting history record: %w", err)
	}
	return res.LastInsertId()
}

// List returns the most recent entries, newest first.
func (s *Store) List(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, prompt, intent_type, intent_json, rendered_cmd, parsed_by, risk, cwd,
		       executed, exit_code, duration_ms, stdout_snippet, stderr_snippet, created_at
		FROM history
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Search returns entries where the prompt contains the query string.
func (s *Store) Search(query string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, prompt, intent_type, intent_json, rendered_cmd, parsed_by, risk, cwd,
		       executed, exit_code, duration_ms, stdout_snippet, stderr_snippet, created_at
		FROM history
		WHERE prompt LIKE ?
		ORDER BY id DESC
		LIMIT ?
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("searching history: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Get retrieves a single entry by ID.
func (s *Store) Get(id int64) (*Entry, error) {
	return s.GetByID(id)
}

// UpdateExecution updates the execution result fields for an existing entry.
func (s *Store) UpdateExecution(id int64, exitCode int, durationMS int64, stdout, stderr string) error {
	_, err := s.db.Exec(`
		UPDATE history
		SET executed = 1, exit_code = ?, duration_ms = ?, stdout_snippet = ?, stderr_snippet = ?
		WHERE id = ?
	`, exitCode, durationMS, utils.RedactString(stdout), utils.RedactString(stderr), id)
	return err
}

// Purge deletes entries older than retentionDays days.
func (s *Store) Purge(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`DELETE FROM history WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// StoreContextChunk stores a retrievable context chunk for RAG-style retrieval.
func (s *Store) StoreContextChunk(source, section, content string, tags []string) error {
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO context_chunks (source, section, content, tags, updated_at)
		VALUES (?,?,?,?,?)
	`, source, section, content, string(tagsJSON), time.Now().UTC().Format(time.RFC3339))
	return err
}

// QueryContextChunks returns context chunks matching any of the given tags.
func (s *Store) QueryContextChunks(tags []string, limit int) ([]ContextChunk, error) {
	if limit <= 0 {
		limit = 10
	}
	// Simple lexical tag matching — LIKE-based approach for MVP without vector DBs.
	var rows *sql.Rows
	var err error

	if len(tags) == 0 {
		rows, err = s.db.Query(`SELECT source, section, content, tags FROM context_chunks LIMIT ?`, limit)
	} else {
		// Build OR-based LIKE clause.
		query := `SELECT source, section, content, tags FROM context_chunks WHERE `
		args := make([]any, 0, len(tags)+1)
		for i, tag := range tags {
			if i > 0 {
				query += " OR "
			}
			query += "tags LIKE ?"
			args = append(args, "%"+tag+"%")
		}
		query += " LIMIT ?"
		args = append(args, limit)
		rows, err = s.db.Query(query, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ContextChunk
	for rows.Next() {
		var c ContextChunk
		var tagsJSON string
		if err := rows.Scan(&c.Source, &c.Section, &c.Content, &tagsJSON); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(tagsJSON), &c.Tags)
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// ContextChunk is a tagged section of a context file used for retrieval.
type ContextChunk struct {
	Source  string   `json:"source"`
	Section string   `json:"section"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// ---- Schema migration --------------------------------------------------------

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			prompt         TEXT    NOT NULL,
			intent_type    TEXT    NOT NULL DEFAULT '',
			intent_json    TEXT    NOT NULL DEFAULT '',
			rendered_cmd   TEXT    NOT NULL DEFAULT '',
			parsed_by      TEXT    NOT NULL DEFAULT '',
			risk           TEXT    NOT NULL DEFAULT '',
			cwd            TEXT    NOT NULL DEFAULT '',
			executed       INTEGER NOT NULL DEFAULT 0,
			exit_code      INTEGER,
			duration_ms    INTEGER NOT NULL DEFAULT 0,
			stdout_snippet TEXT    NOT NULL DEFAULT '',
			stderr_snippet TEXT    NOT NULL DEFAULT '',
			created_at     TEXT    NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_history_created_at ON history(created_at);
		CREATE INDEX IF NOT EXISTS idx_history_intent_type ON history(intent_type);

		CREATE TABLE IF NOT EXISTS context_chunks (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			source     TEXT    NOT NULL,
			section    TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			tags       TEXT    NOT NULL DEFAULT '[]',
			updated_at TEXT    NOT NULL,
			UNIQUE(source, section)
		);

		CREATE INDEX IF NOT EXISTS idx_chunks_source ON context_chunks(source);
	`)
	return err
}

// ---- Helpers -----------------------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		var e Entry
		var createdAt string
		var exitCode sql.NullInt64
		var executed int
		err := rows.Scan(
			&e.ID, &e.Prompt, &e.IntentType, &e.IntentJSON, &e.RenderedCmd,
			&e.ParsedBy, &e.Risk, &e.Cwd,
			&executed, &exitCode, &e.DurationMS,
			&e.StdoutSnippet, &e.StderrSnippet, &createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning history row: %w", err)
		}
		e.Executed = executed == 1
		if exitCode.Valid {
			code := int(exitCode.Int64)
			e.ExitCode = &code
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetByID retrieves a single entry by ID using a query (avoids sql.Row limitations).
func (s *Store) GetByID(id int64) (*Entry, error) {
	rows, err := s.db.Query(`
		SELECT id, prompt, intent_type, intent_json, rendered_cmd, parsed_by, risk, cwd,
		       executed, exit_code, duration_ms, stdout_snippet, stderr_snippet, created_at
		FROM history WHERE id = ? LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("history entry %d not found", id)
	}
	return &entries[0], nil
}
