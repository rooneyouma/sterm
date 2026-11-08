package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
	"github.com/rooneyouma/sterm/internal/model"
	_ "modernc.org/sqlite" // registers the "sqlite" driver; we never call it directly
)

const schema = `
CREATE TABLE IF NOT EXISTS history (
	id        INTEGER PRIMARY KEY,
	cmd       TEXT NOT NULL,
	cwd       TEXT NOT NULL DEFAULT '',
	repo      TEXT NOT NULL DEFAULT '',
	exit_code INTEGER NOT NULL DEFAULT 0,
	ts        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_history_cmd ON history(cmd);
CREATE INDEX IF NOT EXISTS idx_history_ts  ON history(ts);
CREATE UNIQUE INDEX IF NOT EXISTS idx_history_cmd_ts ON history(cmd, ts);
`

const insertSQL = `INSERT OR IGNORE INTO history (cmd, cwd, repo, exit_code, ts) VALUES (?, ?, ?, ?, ?)`

type Store struct {
	db *sql.DB
}

// DefaultPath is where the database lives: ~/.local/share/smart-term/history.db
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "smart-term", "history.db"), nil
}

// Open opens (and creates, if needed) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Insert saves one command.
func (s *Store) Insert(c model.Command) error {
	cmd := strings.TrimRight(c.Cmd, " \t\r\n")
	_, err := s.db.Exec(insertSQL, cmd, c.Cwd, c.Repo, c.ExitCode, c.Time.Unix())
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

// InsertMany saves many commands in one transaction (much faster for imports).
func (s *Store) InsertMany(cmds []model.Command) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, c := range cmds {
		cmd := strings.TrimRight(c.Cmd, " \t\r\n")
		if _, err := stmt.Exec(cmd, c.Cwd, c.Repo, c.ExitCode, c.Time.Unix()); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	}
	return tx.Commit()
}

// Recent returns the newest commands first.
func (s *Store) Recent(limit int) ([]model.Command, error) {
	rows, err := s.db.Query(
		`SELECT id, cmd, cwd, repo, exit_code, ts FROM history ORDER BY ts DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent: %w", err)
	}
	defer rows.Close()

	var out []model.Command
	for rows.Next() {
		var c model.Command
		var ts int64
		if err := rows.Scan(&c.ID, &c.Cmd, &c.Cwd, &c.Repo, &c.ExitCode, &ts); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		c.Time = time.Unix(ts, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}
// PrefixMatches returns distinct commands that start with prefix, best first.
// The exact prefix itself is excluded, since there'd be nothing left to suggest.
func (s *Store) PrefixMatches(prefix string, limit int) ([]model.Match, error) {
	if prefix == "" {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT cmd, COUNT(*) AS n, MAX(ts) AS last
		FROM history
		WHERE substr(cmd, 1, ?) = ? AND cmd != ?
		GROUP BY cmd
		ORDER BY n DESC, last DESC
		LIMIT ?`,
		utf8.RuneCountInString(prefix), prefix, prefix, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("prefix query: %w", err)
	}
	defer rows.Close()

	var out []model.Match
	for rows.Next() {
		var m model.Match
		var last int64
		if err := rows.Scan(&m.Cmd, &m.Count, &last); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		m.Last = time.Unix(last, 0)
		out = append(out, m)
	}
	return out, rows.Err()
}
