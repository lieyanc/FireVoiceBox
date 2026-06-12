// Package store provides SQLite-backed persistence for projects and
// submissions. It uses the pure-Go modernc.org/sqlite driver so the binary
// stays CGO-free and cross-compilable.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id               TEXT PRIMARY KEY,
	slug             TEXT NOT NULL UNIQUE,
	title            TEXT NOT NULL,
	description      TEXT NOT NULL DEFAULT '',
	max_duration_sec INTEGER NOT NULL DEFAULT 60,
	max_per_ip       INTEGER NOT NULL DEFAULT 0,
	status           TEXT NOT NULL DEFAULT 'open',
	manage_token     TEXT NOT NULL,
	created_at       INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS submissions (
	id           TEXT PRIMARY KEY,
	project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	student_id   TEXT NOT NULL DEFAULT '',
	nickname     TEXT NOT NULL DEFAULT '',
	ip           TEXT NOT NULL DEFAULT '',
	user_agent   TEXT NOT NULL DEFAULT '',
	duration_sec INTEGER NOT NULL DEFAULT 0,
	file_path    TEXT NOT NULL,
	mime_type    TEXT NOT NULL DEFAULT '',
	size_bytes   INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_submissions_project ON submissions(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_submissions_project_ip ON submissions(project_id, ip);
`

// Open opens (and migrates) the SQLite database at path.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc's driver is safe for concurrent use, but WAL + a single writer
	// keeps things simple; cap connections to avoid "database is locked".
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }
